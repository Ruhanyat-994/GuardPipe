package dockerx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckBuildStream_SucceedsOnPlainProgress(t *testing.T) {
	stream := strings.NewReader(`{"stream":"Step 1/3 : FROM alpine\n"}` + "\n" +
		`{"stream":" ---> beef\n"}` + "\n")
	require.NoError(t, checkBuildStream(stream))
}

func TestCheckBuildStream_FailsOnErrorDetail(t *testing.T) {
	stream := strings.NewReader(`{"stream":"Step 1/3 : FROM alpine\n"}` + "\n" +
		`{"errorDetail":{"message":"Dockerfile parse error on line 3"},"error":"Dockerfile parse error on line 3"}` + "\n")
	err := checkBuildStream(stream)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Dockerfile parse error on line 3")
}

func TestCheckBuildStream_IgnoresNonJSONLines(t *testing.T) {
	// A malformed or unexpected line must not itself fail the build — only
	// an explicit error field should.
	stream := strings.NewReader("not json at all\n" + `{"stream":"ok\n"}` + "\n")
	require.NoError(t, checkBuildStream(stream))
}
