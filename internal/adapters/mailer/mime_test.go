package mailer

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
)

// TestBuildMessage_RoundTrips parses the built message back with the
// standard library, the way a receiving mail server would.
func TestBuildMessage_RoundTrips(t *testing.T) {
	pdf := bytes.Repeat([]byte("%PDF-1.7 report body "), 200)
	raw, err := buildMessage("GuardPipe <reports@example.com>", notification.Email{
		To:      "owner@example.com",
		Subject: "[GuardPipe] Zahlungs-API · Scan #4 — risk score 72 (warn)",
		Text:    "plain body",
		HTML:    "<p>html body</p>",
		Attachments: []notification.Attachment{
			{Filename: "guardpipe-scan-4.pdf", ContentType: "application/pdf", Data: pdf},
		},
	})
	require.NoError(t, err)

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, "owner@example.com", msg.Header.Get("To"))
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "[GuardPipe] Zahlungs-API · Scan #4 — risk score 72 (warn)", subject)

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/mixed", mediaType)

	mr := multipart.NewReader(msg.Body, params["boundary"])
	alt, err := mr.NextPart()
	require.NoError(t, err)
	altType, altParams, err := mime.ParseMediaType(alt.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/alternative", altType)
	ar := multipart.NewReader(alt, altParams["boundary"])
	var bodies []string
	for {
		p, err := ar.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		enc, err := io.ReadAll(p)
		require.NoError(t, err)
		b, err := decodeBase64Lines(string(enc))
		require.NoError(t, err)
		bodies = append(bodies, string(b))
	}
	require.Equal(t, []string{"plain body", "<p>html body</p>"}, bodies)

	att, err := mr.NextPart()
	require.NoError(t, err)
	require.Equal(t, "guardpipe-scan-4.pdf", att.FileName())
	// mime/multipart only auto-decodes quoted-printable, so base64 parts
	// are decoded here.
	enc, err := io.ReadAll(att)
	require.NoError(t, err)
	decoded, err := decodeBase64Lines(string(enc))
	require.NoError(t, err)
	require.Equal(t, pdf, decoded)

	_, err = mr.NextPart()
	require.Equal(t, io.EOF, err)
}

// Near-miss: a header value carrying a line break must fail rather than
// inject a header (e.g. a Bcc).
func TestBuildMessage_RejectsHeaderInjection(t *testing.T) {
	_, err := buildMessage("GuardPipe <reports@example.com>", notification.Email{To: "a@example.com\r\nBcc: x@evil.test", Subject: "s"})
	require.Error(t, err)
	_, err = buildMessage("GuardPipe <reports@example.com>\nBcc: x@evil.test", notification.Email{To: "a@example.com", Subject: "s"})
	require.Error(t, err)
	_, err = buildMessage("GuardPipe <reports@example.com>", notification.Email{To: "a@example.com", Attachments: []notification.Attachment{{Filename: "a.pdf\r\nX: y", ContentType: "application/pdf"}}})
	require.Error(t, err)
}

// A subject with a line break can't smuggle a header either: Q-encoding
// turns it into encoded text inside the one Subject line.
func TestBuildMessage_SubjectLineBreakStaysInSubject(t *testing.T) {
	raw, err := buildMessage("GuardPipe <reports@example.com>", notification.Email{To: "a@example.com", Subject: "hi\r\nBcc: x@evil.test", Text: "t"})
	require.NoError(t, err)
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Empty(t, msg.Header.Get("Bcc"))
}

func decodeBase64Lines(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.NewReplacer("\r", "", "\n", "").Replace(s))
}
