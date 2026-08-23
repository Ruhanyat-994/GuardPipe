package validate_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
)

// fakeResolver lets tests script DNS answers deterministically instead of
// depending on the real network (documentation's testing philosophy: no
// live external calls).
type fakeResolver map[string][]net.IPAddr

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addrs, ok := f[host]
	if !ok {
		return nil, errors.New("fakeResolver: no such host")
	}
	return addrs, nil
}

func ipAddrs(ips ...string) []net.IPAddr {
	out := make([]net.IPAddr, len(ips))
	for i, s := range ips {
		out[i] = net.IPAddr{IP: net.ParseIP(s)}
	}
	return out
}

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		resolver     fakeResolver
		allowPrivate bool
		denylist     []string
		wantHost     string
		wantErr      error
	}{
		{
			// True positive: any public host not on the denylist resolves
			// and passes — there is no allowlist to be on.
			name:     "public host not on denylist is accepted",
			target:   "https://staging.acme.example/health",
			resolver: fakeResolver{"staging.acme.example": ipAddrs("203.0.113.10")},
			wantHost: "staging.acme.example",
		},
		{
			// Near-miss: same kind of public address, but the host is on
			// the denylist — must fire even though the range check passed.
			name:     "public host on denylist is blocked",
			target:   "https://evil.example",
			resolver: fakeResolver{"evil.example": ipAddrs("203.0.113.20")},
			denylist: []string{"evil.example"},
			wantErr:  validate.ErrTargetBlocked,
		},
		{
			name:     "RFC1918 private address is blocked by default",
			target:   "internal.acme.example",
			resolver: fakeResolver{"internal.acme.example": ipAddrs("192.168.1.50")},
			wantErr:  validate.ErrTargetBlocked,
		},
		{
			name:     "loopback address is blocked",
			target:   "localhost.acme.example",
			resolver: fakeResolver{"localhost.acme.example": ipAddrs("127.0.0.1")},
			wantErr:  validate.ErrTargetBlocked,
		},
		{
			name:     "cloud metadata address is blocked",
			target:   "metadata.acme.example",
			resolver: fakeResolver{"metadata.acme.example": ipAddrs("169.254.169.254")},
			wantErr:  validate.ErrTargetBlocked,
		},
		{
			name:     "IPv6 unique-local address is blocked",
			target:   "ula.acme.example",
			resolver: fakeResolver{"ula.acme.example": ipAddrs("fd00::1")},
			wantErr:  validate.ErrTargetBlocked,
		},
		{
			// Near-miss: private range explicitly permitted by config —
			// must not fire the block.
			name:         "private address permitted when allowPrivate is set",
			target:       "internal.acme.example",
			resolver:     fakeResolver{"internal.acme.example": ipAddrs("192.168.1.50")},
			allowPrivate: true,
			wantHost:     "internal.acme.example",
		},
		{
			name:     "unresolvable host is rejected",
			target:   "ghost.acme.example",
			resolver: fakeResolver{},
			wantErr:  validate.ErrTargetUnresolvable,
		},
		{
			name:     "malformed target is rejected",
			target:   "   ",
			resolver: fakeResolver{},
			wantErr:  validate.ErrTargetMalformed,
		},
		{
			// Near-miss: a similarly-named domain must not match a denylist
			// entry via naive substring matching.
			name:     "lookalike domain does not match denylist suffix",
			target:   "https://notevil.example",
			resolver: fakeResolver{"notevil.example": ipAddrs("203.0.113.30")},
			denylist: []string{"evil.example"},
			wantHost: "notevil.example",
		},
		{
			// True positive: a subdomain of a denylisted host is still
			// blocked via suffix matching.
			name:     "subdomain of denylisted host is blocked",
			target:   "https://sub.evil.example",
			resolver: fakeResolver{"sub.evil.example": ipAddrs("203.0.113.40")},
			denylist: []string{"evil.example"},
			wantErr:  validate.ErrTargetBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validate.ResolveTarget(context.Background(), tt.resolver, tt.target, tt.allowPrivate, tt.denylist)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ResolveTarget() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTarget() unexpected error: %v", err)
			}
			if got.NormalizedHost != tt.wantHost {
				t.Errorf("NormalizedHost = %q, want %q", got.NormalizedHost, tt.wantHost)
			}
			if len(got.PinnedIPs) == 0 {
				t.Errorf("PinnedIPs is empty, want at least one address")
			}
		})
	}
}
