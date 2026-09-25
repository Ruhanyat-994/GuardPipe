package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// signaturePrefix is the scheme GitHub puts in front of the hex digest in
// the X-Hub-Signature-256 header.
const signaturePrefix = "sha256="

// VerifySignature reports whether header (the raw X-Hub-Signature-256
// value) is a valid HMAC-SHA256 of body under secret. The comparison is
// constant-time (hmac.Equal) so response timing can't be used to guess a
// valid signature byte by byte — documentation/12-security-and-threat-model.md
// risk S4, forged webhook.
func VerifySignature(secret, body []byte, header string) bool {
	if len(secret) == 0 || !strings.HasPrefix(header, signaturePrefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, signaturePrefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// Sign returns the X-Hub-Signature-256 header value for body under secret —
// the inverse of VerifySignature. Used by tests, and handy for sending a
// correctly-signed request to a local instance by hand.
func Sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// PushEvent is the subset of GitHub's `push` webhook payload live scanning
// reads (https://docs.github.com/en/webhooks/webhook-events-and-payloads#push).
type PushEvent struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// Branch is the pushed branch's short name, or "" when the push was to
// something other than a branch (a tag).
func (e PushEvent) Branch() string {
	const prefix = "refs/heads/"
	if !strings.HasPrefix(e.Ref, prefix) {
		return ""
	}
	return strings.TrimPrefix(e.Ref, prefix)
}

// PullRequestEvent is the subset of GitHub's `pull_request` webhook payload
// live scanning reads
// (https://docs.github.com/en/webhooks/webhook-events-and-payloads#pull_request).
type PullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Head struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// FromFork reports whether the PR's head branch lives in a different
// repository (a fork). That branch can't be cloned from this repository's
// URL, and its code comes from someone outside the repository.
func (e PullRequestEvent) FromFork() bool {
	return !strings.EqualFold(e.PullRequest.Head.Repo.FullName, e.Repository.FullName)
}

// ParsePushEvent decodes a `push` payload.
func ParsePushEvent(body []byte) (*PushEvent, error) {
	var e PushEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("github: decode push event: %w", err)
	}
	return &e, nil
}

// ParsePullRequestEvent decodes a `pull_request` payload.
func ParsePullRequestEvent(body []byte) (*PullRequestEvent, error) {
	var e PullRequestEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("github: decode pull_request event: %w", err)
	}
	return &e, nil
}
