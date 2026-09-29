package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type Sender interface {
	Send(context.Context, Message) error
}

type SMTPConfig struct {
	Addr     string
	From     string
	Username string
	Password string
	Timeout  time.Duration
}

type SMTPSender struct {
	config      SMTPConfig
	from        *stdmail.Address
	host        string
	port        int
	implicitTLS bool
}

func NewSMTPSender(config SMTPConfig) (*SMTPSender, error) {
	config.Addr = strings.TrimSpace(config.Addr)
	host, portText, err := net.SplitHostPort(config.Addr)
	if err != nil {
		return nil, fmt.Errorf("parse SMTP address: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid SMTP port %q", portText)
	}
	from, err := stdmail.ParseAddress(config.From)
	if err != nil {
		return nil, fmt.Errorf("parse SMTP sender: %w", err)
	}
	if config.Timeout <= 0 {
		config.Timeout = 15 * time.Second
	}
	return &SMTPSender{
		config:      config,
		from:        from,
		host:        host,
		port:        port,
		implicitTLS: port == 465,
	}, nil
}

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	if err := message.validate(); err != nil {
		return err
	}
	recipient, err := stdmail.ParseAddress(message.To)
	if err != nil {
		return fmt.Errorf("parse recipient: %w", err)
	}
	payload, err := encodeMessage(s.from, recipient, message)
	if err != nil {
		return err
	}

	dialer := net.Dialer{Timeout: s.config.Timeout}
	var connection net.Conn
	if s.implicitTLS {
		tlsDialer := tls.Dialer{
			NetDialer: &dialer,
			Config: &tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: s.host,
			},
		}
		connection, err = tlsDialer.DialContext(ctx, "tcp", s.config.Addr)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", s.config.Addr)
	}
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer connection.Close()

	deadline := time.Now().Add(s.config.Timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}

	client, err := smtp.NewClient(connection, s.host)
	if err != nil {
		return fmt.Errorf("start SMTP client: %w", err)
	}
	defer client.Close()

	if !s.implicitTLS {
		if supported, _ := client.Extension("STARTTLS"); supported {
			if err := client.StartTLS(&tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: s.host,
			}); err != nil {
				return fmt.Errorf("start SMTP TLS: %w", err)
			}
		}
	}
	if s.config.Username != "" {
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticate to SMTP server: %w", err)
		}
	}
	if err := client.Mail(s.from.Address); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP body: %w", err)
	}
	if _, err := writer.Write(payload); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP body: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("finish SMTP session: %w", err)
	}
	return nil
}

func encodeMessage(from, recipient *stdmail.Address, message Message) ([]byte, error) {
	var body bytes.Buffer
	writeHeader := func(name, value string) {
		_, _ = fmt.Fprintf(&body, "%s: %s\r\n", name, value)
	}
	writeHeader("From", from.String())
	writeHeader("To", recipient.String())
	writeHeader("Subject", mime.QEncoding.Encode("UTF-8", message.Subject))
	writeHeader("Date", time.Now().Format(time.RFC1123Z))
	writeHeader("MIME-Version", "1.0")

	if message.HTMLBody == "" {
		writeHeader("Content-Type", `text/plain; charset="UTF-8"`)
		writeHeader("Content-Transfer-Encoding", "quoted-printable")
		body.WriteString("\r\n")
		quoted := quotedprintable.NewWriter(&body)
		if _, err := io.WriteString(quoted, normalizeNewlines(message.TextBody)); err != nil {
			return nil, fmt.Errorf("encode email text: %w", err)
		}
		if err := quoted.Close(); err != nil {
			return nil, fmt.Errorf("finish email text: %w", err)
		}
		return body.Bytes(), nil
	}

	multipartWriter := multipart.NewWriter(&body)
	writeHeader("Content-Type", `multipart/alternative; boundary="`+multipartWriter.Boundary()+`"`)
	body.WriteString("\r\n")
	if err := writeMIMEPart(multipartWriter, "text/plain", message.TextBody); err != nil {
		return nil, err
	}
	if err := writeMIMEPart(multipartWriter, "text/html", message.HTMLBody); err != nil {
		return nil, err
	}
	if err := multipartWriter.Close(); err != nil {
		return nil, fmt.Errorf("finish MIME email: %w", err)
	}
	return body.Bytes(), nil
}

func writeMIMEPart(writer *multipart.Writer, contentType, value string) error {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", contentType+`; charset="UTF-8"`)
	header.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create %s email part: %w", contentType, err)
	}
	quoted := quotedprintable.NewWriter(part)
	if _, err := io.WriteString(quoted, normalizeNewlines(value)); err != nil {
		return fmt.Errorf("encode %s email part: %w", contentType, err)
	}
	if err := quoted.Close(); err != nil {
		return fmt.Errorf("finish %s email part: %w", contentType, err)
	}
	return nil
}

func normalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}
