package vcs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/vcs"
)

type fakeGitHubClient struct {
	meta *github.RepoMetadata
	err  error
}

func (f *fakeGitHubClient) GetRepository(_ context.Context, owner, name, token string) (*github.RepoMetadata, error) {
	return f.meta, f.err
}

func TestService_ValidateRepository_Success(t *testing.T) {
	client := &fakeGitHubClient{meta: &github.RepoMetadata{DefaultBranch: "main", IsPrivate: true, SizeKB: 4096}}
	svc := vcs.NewService(client, nil, 500)

	info, err := svc.ValidateRepository(context.Background(), "https://github.com/acme/payments-api", "ghp_token")
	require.NoError(t, err)
	require.Equal(t, "acme", info.Owner)
	require.Equal(t, "payments-api", info.Name)
	require.Equal(t, "https://github.com/acme/payments-api", info.NormalizedURL)
	require.Equal(t, "main", info.DefaultBranch)
	require.True(t, info.IsPrivate)
	require.EqualValues(t, 4096, info.SizeKB)
}

func TestService_ValidateRepository_InvalidURLNeverCallsGitHub(t *testing.T) {
	client := &fakeGitHubClient{err: errors.New("must not be called")}
	svc := vcs.NewService(client, nil, 500)

	_, err := svc.ValidateRepository(context.Background(), "https://gitlab.com/acme/payments-api", "")
	require.ErrorIs(t, err, github.ErrInvalidRepositoryURL)
}

func TestService_ValidateRepository_PropagatesClientError(t *testing.T) {
	wantErr := errors.New("boom")
	client := &fakeGitHubClient{err: wantErr}
	svc := vcs.NewService(client, nil, 500)

	_, err := svc.ValidateRepository(context.Background(), "https://github.com/acme/payments-api", "")
	require.ErrorIs(t, err, wantErr)
}

func TestService_ShallowClone_UsesNormalizedCloneURLAndByteCap(t *testing.T) {
	var gotURL, gotToken, gotDest string
	var gotMax int64
	clone := func(_ context.Context, cloneURL, token, destDir string, maxBytes int64) error {
		gotURL, gotToken, gotDest, gotMax = cloneURL, token, destDir, maxBytes
		return nil
	}
	svc := vcs.NewService(nil, clone, 250)

	err := svc.ShallowClone(context.Background(), "https://github.com/acme/payments-api.git", "ghp_token", "/tmp/ws/scan-1")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/acme/payments-api.git", gotURL)
	require.Equal(t, "ghp_token", gotToken)
	require.Equal(t, "/tmp/ws/scan-1", gotDest)
	require.EqualValues(t, 250*1024*1024, gotMax)
}

func TestService_ShallowClone_InvalidURLNeverCallsClone(t *testing.T) {
	clone := func(context.Context, string, string, string, int64) error {
		t.Fatal("clone must not be called for an invalid URL")
		return nil
	}
	svc := vcs.NewService(nil, clone, 250)

	err := svc.ShallowClone(context.Background(), "not a url", "", "/tmp/ws")
	require.ErrorIs(t, err, github.ErrInvalidRepositoryURL)
}
