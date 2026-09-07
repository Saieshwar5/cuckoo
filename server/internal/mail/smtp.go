package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// sendTimeout bounds one delivery. A provider that has stopped answering
// must not hold a person's sign-in request open.
const sendTimeout = 20 * time.Second

// SMTP sends through a mail provider.
//
// Two shapes of connection exist in the wild and both are here: port 465
// wraps the whole session in TLS, and port 587 starts in the clear and
// upgrades with STARTTLS. Anything else is refused rather than sent, because
// a sign-in code in the clear is a sign-in code anyone on the path can read.
type SMTP struct {
	addr     string
	host     string
	auth     smtp.Auth
	from     string
	fromName string
	implicit bool
	// tlsCfg is how the connection is secured. A test replaces it to trust
	// its own server's certificate; nothing else touches it.
	tlsCfg *tls.Config
}

// SMTPConfig is what the hub is told about its provider.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the address messages come from. It must be one the provider
	// has been told this hub may use, or everything is refused at send time.
	From string
	// FromName is the name beside the address, e.g. "Cuckoo".
	FromName string
}

// NewSMTP builds an SMTP mailer.
func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	if cfg.Host == "" {
		return nil, errors.New("smtp: host is empty")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("smtp: from address %q: %w", cfg.From, err)
	}
	switch cfg.Port {
	case 465, 587, 2587:
	default:
		return nil, fmt.Errorf("smtp: port %d is not one this hub will use; 587 (STARTTLS) or 465 (TLS)", cfg.Port)
	}
	s := &SMTP{
		addr:     net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port)),
		host:     cfg.Host,
		from:     cfg.From,
		fromName: cfg.FromName,
		implicit: cfg.Port == 465,
	}
	s.tlsCfg = &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	if cfg.Username != "" {
		s.auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	return s, nil
}

// Send implements Mailer.
func (s *SMTP) Send(ctx context.Context, m Message) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	client, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if !s.implicit {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp: server does not offer STARTTLS; refusing to send a code in the clear")
		}
		if err := client.StartTLS(s.tlsCfg); err != nil {
			return fmt.Errorf("smtp: start tls: %w", err)
		}
	}
	if s.auth != nil {
		if err := client.Auth(s.auth); err != nil {
			return fmt.Errorf("smtp: authenticate: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("smtp: from: %w", err)
	}
	if err := client.Rcpt(m.To); err != nil {
		return fmt.Errorf("smtp: to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(s.compose(m)); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: finish: %w", err)
	}
	return client.Quit()
}

// dial opens the connection, honouring the context's deadline so a provider
// that accepts a connection and then says nothing still fails in time.
func (s *SMTP) dial(ctx context.Context) (*smtp.Client, error) {
	dialer := &net.Dialer{}

	var (
		conn net.Conn
		err  error
	)
	if s.implicit {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: s.tlsCfg}).DialContext(ctx, "tcp", s.addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", s.addr)
	}
	if err != nil {
		return nil, fmt.Errorf("smtp: dial %s: %w", s.addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp: greet %s: %w", s.addr, err)
	}
	return client, nil
}

// compose writes the message. Plain text, no links: a code that is typed in
// cannot be phished by a lookalike address, which is the reason for choosing
// codes over magic links in the first place.
func (s *SMTP) compose(m Message) []byte {
	from := s.from
	if s.fromName != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", s.fromName), s.from)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("\r\n")
	// The writer the client hands back escapes a line that begins with a
	// dot, which would otherwise end the message early, so the text goes in
	// as it is with only its line endings normalised.
	for line := range strings.SplitSeq(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n") {
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}
