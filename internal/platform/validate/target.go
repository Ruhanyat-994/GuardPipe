package validate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrTargetMalformed means the input string is neither a bare hostname nor a
// parseable URL.
var ErrTargetMalformed = errors.New("validate: target is not a valid hostname or URL")

// ErrTargetUnresolvable means DNS returned no A/AAAA records at all.
var ErrTargetUnresolvable = errors.New("validate: target host does not resolve to any address")

// ErrTargetBlocked means every resolved address (or the only one checked)
// falls in a blocked range, or the host isn't covered by the allowlist.
var ErrTargetBlocked = errors.New("validate: target address is blocked")

// targetErr carries a caller-facing Error() string that stays clean of the
// "validate: …" package prefix — callers use it directly as an RFC 9457
// `detail` (documentation/07-api-specification.md §1.3's own example is
// exactly this wording: "staging.acme.example resolves to 192.168.1.50,
// which is in a blocked private range") — while errors.Is still matches the
// sentinel via Unwrap.
type targetErr struct {
	sentinel error
	detail   string
}

func (e *targetErr) Error() string { return e.detail }
func (e *targetErr) Unwrap() error { return e.sentinel }

// TargetResolution is a validated pentest target
// (documentation/05-module-specifications.md §4, "Target validation").
type TargetResolution struct {
	NormalizedHost string
	PinnedIPs      []net.IP
}

// Resolver is the subset of *net.Resolver ResolveTarget needs — satisfied by
// net.DefaultResolver, and by a fake in tests so DNS behaviour is
// deterministic rather than dependent on the network.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// ResolveTarget implements the flowchart in
// documentation/05-module-specifications.md §4: parse → DNS resolve → reject
// private/loopback/link-local/multicast/metadata addresses unless
// allowPrivate → require an allowlist match. allowlist entries match a host
// exactly or as a suffix (so "example.com" also matches
// "staging.example.com"); a nil/empty allowlist matches nothing — a pentest
// target must be explicitly allowlisted, there is no "allow everything"
// default for a feature that launches real network probes.
func ResolveTarget(ctx context.Context, resolver Resolver, target string, allowPrivate bool, allowlist []string) (*TargetResolution, error) {
	host, err := normalizeTargetHost(target)
	if err != nil {
		return nil, err
	}

	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, &targetErr{sentinel: ErrTargetUnresolvable, detail: fmt.Sprintf("%s does not resolve to any address", host)}
	}

	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}

	if !allowPrivate {
		for _, ip := range ips {
			if isBlockedRange(ip) {
				return nil, &targetErr{sentinel: ErrTargetBlocked, detail: fmt.Sprintf("%s resolves to %s, which is in a blocked private range", host, ip.String())}
			}
		}
	}

	if !hostAllowlisted(host, allowlist) {
		return nil, &targetErr{sentinel: ErrTargetBlocked, detail: fmt.Sprintf("%s is not in the pentest target allowlist", host)}
	}

	return &TargetResolution{NormalizedHost: host, PinnedIPs: ips}, nil
}

// normalizeTargetHost accepts either a bare hostname ("staging.acme.example")
// or a URL ("https://staging.acme.example/path") and returns the lowercase
// hostname alone, with no scheme, path, or port.
func normalizeTargetHost(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", &targetErr{sentinel: ErrTargetMalformed, detail: "target must not be empty"}
	}

	candidate := target
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return "", &targetErr{sentinel: ErrTargetMalformed, detail: fmt.Sprintf("%q is not a valid hostname or URL", target)}
		}
		candidate = u.Hostname()
	} else if h, _, err := net.SplitHostPort(target); err == nil {
		candidate = h
	}

	candidate = strings.ToLower(strings.Trim(candidate, "."))
	if candidate == "" {
		return "", &targetErr{sentinel: ErrTargetMalformed, detail: fmt.Sprintf("%q is not a valid hostname or URL", target)}
	}
	return candidate, nil
}

// isBlockedRange covers every range named in
// documentation/05-module-specifications.md §4: 10/8, 172.16/12,
// 192.168/16, 127/8, ::1, 169.254/16 (including the 169.254.169.254 cloud
// metadata address), fc00::/7, and 0.0.0.0 — net.IP's own classifiers cover
// all of them without hand-rolling CIDR tables.
func isBlockedRange(ip net.IP) bool {
	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// hostAllowlisted matches host against allowlist entries exactly or as a
// dot-boundary suffix, so an entry of "acme.example" also allows
// "staging.acme.example" without allowing "evilacme.example".
func hostAllowlisted(host string, allowlist []string) bool {
	for _, entry := range allowlist {
		entry = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(entry, "*.")))
		if entry == "" {
			continue
		}
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}
