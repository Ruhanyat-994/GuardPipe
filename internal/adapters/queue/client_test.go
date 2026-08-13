package queue_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/queue"
)

func TestNew_ValidURL(t *testing.T) {
	client, err := queue.New("redis://localhost:6379/0")
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()
}

func TestNew_InvalidURL(t *testing.T) {
	_, err := queue.New("not-a-redis-url")
	require.Error(t, err)
}
