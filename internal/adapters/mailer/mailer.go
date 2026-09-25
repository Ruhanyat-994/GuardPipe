package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
)

// Log "sends" by writing one log line — never the body or attachment, which
// hold security findings.
type Log struct {
	Logger *slog.Logger
}

func (l Log) Send(_ context.Context, e notification.Email) error {
	size := 0
	for _, a := range e.Attachments {
		size += len(a.Data)
	}
	l.Logger.Info("mailer(log): email not sent — GUARDPIPE_MAIL_BACKEND=log",
		"to", e.To, "subject", e.Subject, "attachments", len(e.Attachments), "attachment_bytes", size)
	return nil
}

// SMTP sends through an SMTP server. Locally that's Mailpit (no auth, no
// TLS); anything else should offer STARTTLS, which is used whenever the
// server advertises it. Auth is only attempted when a username is set.
type SMTP struct {
	Addr     string // host:port
	From     string
	Username string
	Password string
}

func (s SMTP) Send(ctx context.Context, e notification.Email) error {
	msg, err := buildMessage(s.From, e)
	if err != nil {
		return err
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("mailer: invalid from address: %w", err)
	}

	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("mailer: invalid SMTP address %q: %w", s.Addr, err)
	}
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("mailer: connect to SMTP server: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(time.Minute))
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mailer: SMTP handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mailer: STARTTLS: %w", err)
		}
	}
	if s.Username != "" {
		// smtp.PlainAuth itself refuses to send credentials over an
		// unencrypted connection to anything but localhost.
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return fmt.Errorf("mailer: SMTP auth: %w", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(e.To); err != nil {
		return fmt.Errorf("mailer: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mailer: write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: finish message: %w", err)
	}
	return c.Quit()
}

// SES sends through Amazon SES (API v2, SendEmail with raw content, which
// is what allows attachments). Credentials come from the AWS SDK's default
// chain: the pod's IAM role (IRSA) on EKS, or a developer's own AWS profile
// locally — never a key in GuardPipe's config.
type SES struct {
	client *sesv2.Client
	from   string
	// configurationSet is optional (SES event publishing: bounces,
	// complaints). Empty = none.
	configurationSet string
}

// NewSES builds an SES mailer for region.
func NewSES(ctx context.Context, region, from, configurationSet string) (*SES, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("mailer: load AWS config: %w", err)
	}
	return &SES{client: sesv2.NewFromConfig(cfg), from: from, configurationSet: configurationSet}, nil
}

func (s *SES) Send(ctx context.Context, e notification.Email) error {
	msg, err := buildMessage(s.from, e)
	if err != nil {
		return err
	}
	in := &sesv2.SendEmailInput{
		Destination: &sestypes.Destination{ToAddresses: []string{e.To}},
		Content:     &sestypes.EmailContent{Raw: &sestypes.RawMessage{Data: msg}},
	}
	if s.configurationSet != "" {
		in.ConfigurationSetName = aws.String(s.configurationSet)
	}
	if _, err := s.client.SendEmail(ctx, in); err != nil {
		return fmt.Errorf("mailer: SES SendEmail: %w", err)
	}
	return nil
}
