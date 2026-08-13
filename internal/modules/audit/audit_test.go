package audit_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

type fakeRepo struct {
	inserted []audit.Entry
	err      error
}

func (f *fakeRepo) Insert(_ context.Context, e audit.Entry) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = append(f.inserted, e)
	return nil
}

func TestLog_InsertsEntry(t *testing.T) {
	repo := &fakeRepo{}
	svc := audit.NewService(repo, nil)

	actorID := id.New()
	svc.Log(context.Background(), audit.Entry{ActorID: &actorID, Action: "auth.login"})

	require.Len(t, repo.inserted, 1)
	require.Equal(t, "auth.login", repo.inserted[0].Action)
	require.Equal(t, actorID, *repo.inserted[0].ActorID)
}

func TestLog_RepositoryFailure_NeverPanicsOrBlocksCaller(t *testing.T) {
	repo := &fakeRepo{err: errors.New("connection reset")}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	svc := audit.NewService(repo, logger)

	require.NotPanics(t, func() {
		svc.Log(context.Background(), audit.Entry{Action: "auth.login"})
	})
	require.Contains(t, buf.String(), "audit: failed to record event")
	require.Contains(t, buf.String(), "auth.login")
}
