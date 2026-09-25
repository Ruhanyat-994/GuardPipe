// Package mailer implements notification.Mailer three ways, picked by
// GUARDPIPE_MAIL_BACKEND: "log" (writes a line instead of sending — tests,
// CI, and any environment without email), "smtp" (any SMTP server; Mailpit
// in docker-compose for local development), and "ses" (Amazon SES through
// the AWS SDK, authenticated by the pod's IAM role — no static keys).
//
// SMTP and SES both send the same raw RFC 5322 message built here, so what
// you see in Mailpit locally is byte-for-byte what SES delivers.
package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
)

// buildMessage renders e as a complete MIME message from `from`:
// multipart/mixed { multipart/alternative { text, html }, attachments... }.
func buildMessage(from string, e notification.Email) ([]byte, error) {
	if err := checkHeader("from", from); err != nil {
		return nil, err
	}
	if err := checkHeader("to", e.To); err != nil {
		return nil, err
	}
	if _, err := mail.ParseAddress(e.To); err != nil {
		return nil, fmt.Errorf("mailer: invalid recipient: %w", err)
	}

	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)
	alt := multipart.NewWriter(nil) // only for its random boundary
	altBoundary := alt.Boundary()

	headers := []struct{ k, v string }{
		{"From", from},
		{"To", e.To},
		// Q-encoding handles non-ASCII project names and can't carry a raw
		// CR/LF, so a subject can never smuggle in extra headers.
		{"Subject", mime.QEncoding.Encode("utf-8", e.Subject)},
		{"Date", time.Now().UTC().Format(time.RFC1123Z)},
		{"Message-ID", messageID(from)},
		{"MIME-Version", "1.0"},
		{"Content-Type", `multipart/mixed; boundary="` + mixed.Boundary() + `"`},
	}
	var head bytes.Buffer
	for _, h := range headers {
		fmt.Fprintf(&head, "%s: %s\r\n", h.k, h.v)
	}
	head.WriteString("\r\n")

	altPart, err := mixed.CreatePart(textproto.MIMEHeader{
		"Content-Type": {`multipart/alternative; boundary="` + altBoundary + `"`},
	})
	if err != nil {
		return nil, err
	}
	altWriter := multipart.NewWriter(altPart)
	if err := altWriter.SetBoundary(altBoundary); err != nil {
		return nil, err
	}
	for _, body := range []struct{ ctype, content string }{
		{"text/plain; charset=utf-8", e.Text},
		{"text/html; charset=utf-8", e.HTML},
	} {
		if body.content == "" {
			continue
		}
		w, err := altWriter.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {body.ctype},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, err
		}
		if err := writeBase64(w, []byte(body.content)); err != nil {
			return nil, err
		}
	}
	if err := altWriter.Close(); err != nil {
		return nil, err
	}

	for _, a := range e.Attachments {
		if err := checkHeader("attachment filename", a.Filename); err != nil {
			return nil, err
		}
		w, err := mixed.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType(a.ContentType, map[string]string{"name": a.Filename})},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename})},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, err
		}
		if err := writeBase64(w, a.Data); err != nil {
			return nil, err
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return append(head.Bytes(), buf.Bytes()...), nil
}

// writeBase64 writes data base64-encoded in 76-character lines (RFC 2045).
func writeBase64(w interface{ Write([]byte) (int, error) }, data []byte) error {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		if _, err := w.Write([]byte(enc[:76] + "\r\n")); err != nil {
			return err
		}
		enc = enc[76:]
	}
	_, err := w.Write([]byte(enc + "\r\n"))
	return err
}

func checkHeader(name, v string) error {
	if strings.ContainsAny(v, "\r\n") {
		return fmt.Errorf("mailer: %s contains a line break", name)
	}
	return nil
}

func messageID(from string) string {
	domain := "guardpipe.local"
	if addr, err := mail.ParseAddress(from); err == nil {
		if at := strings.LastIndex(addr.Address, "@"); at >= 0 {
			domain = addr.Address[at+1:]
		}
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "<" + hex.EncodeToString(b) + "@" + domain + ">"
}
