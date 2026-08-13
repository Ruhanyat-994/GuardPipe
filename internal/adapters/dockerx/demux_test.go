package dockerx

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// frame builds one stdcopy frame: streamType (1=stdout, 2=stderr) followed
// by len(payload) big-endian and the payload itself.
func frame(streamType byte, payload string) []byte {
	header := make([]byte, 8)
	header[0] = streamType
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	return append(header, []byte(payload)...)
}

func TestDemuxLogs_SeparatesStdoutAndStderr(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(frame(1, "hello stdout\n"))
	buf.Write(frame(2, "oops stderr\n"))
	buf.Write(frame(1, "more stdout\n"))

	stdout, stderr, err := demuxLogs(&buf)
	require.NoError(t, err)
	require.Equal(t, "hello stdout\nmore stdout\n", string(stdout))
	require.Equal(t, "oops stderr\n", string(stderr))
}

func TestDemuxLogs_EmptyStream(t *testing.T) {
	stdout, stderr, err := demuxLogs(&bytes.Buffer{})
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
}

func TestDemuxLogs_IgnoresStdinFrames(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(frame(0, "should be ignored"))
	buf.Write(frame(1, "kept"))

	stdout, stderr, err := demuxLogs(&buf)
	require.NoError(t, err)
	require.Equal(t, "kept", string(stdout))
	require.Empty(t, stderr)
}

func TestDemuxLogs_TruncatedHeader_Errors(t *testing.T) {
	_, _, err := demuxLogs(bytes.NewReader([]byte{1, 0, 0}))
	require.Error(t, err)
}
