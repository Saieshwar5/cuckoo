package mail_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/mail"
)

// fakeSMTP is a server that speaks just enough of the protocol to accept one
// message, so the client is tested against a conversation rather than a mock.
type fakeSMTP struct {
	mu       sync.Mutex
	received string
	from     string
	to       string
	authed   bool
	tlsCfg   *tls.Config
	// noSTARTTLS makes the server hide the extension, which the client must
	// refuse rather than send a code in the clear.
	noSTARTTLS bool
}

func (f *fakeSMTP) body() (string, string, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.received, f.from, f.to, f.authed
}

func (f *fakeSMTP) serve(t *testing.T, conn net.Conn) {
	t.Helper()
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	say := func(s string) {
		_, _ = rw.WriteString(s + "\r\n")
		_ = rw.Flush()
	}
	say("220 fake ESMTP")
	for {
		line, err := rw.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			if f.noSTARTTLS {
				say("250-fake")
				say("250 AUTH PLAIN")
				continue
			}
			say("250-fake")
			say("250-STARTTLS")
			say("250 AUTH PLAIN")
		case cmd == "STARTTLS":
			say("220 go ahead")
			tlsConn := tls.Server(conn, f.tlsCfg)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		case strings.HasPrefix(cmd, "AUTH"):
			f.mu.Lock()
			f.authed = true
			f.mu.Unlock()
			say("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			f.mu.Lock()
			f.from = strings.TrimSpace(line)
			f.mu.Unlock()
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO"):
			f.mu.Lock()
			f.to = strings.TrimSpace(line)
			f.mu.Unlock()
			say("250 ok")
		case cmd == "DATA":
			say("354 send it")
			var b strings.Builder
			for {
				l, err := rw.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.received = b.String()
			f.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

// start runs the fake on a port the test is told, with a certificate the
// client is told to trust.
func start(t *testing.T, f *fakeSMTP) (host string, port int) {
	t.Helper()
	cert := selfSigned(t)
	f.tlsCfg = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(t, conn)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestSMTPRejectsWhatItCannotSendSafely(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  mail.SMTPConfig
	}{
		{"no host", mail.SMTPConfig{Port: 587, From: "a@b.test"}},
		{"no from", mail.SMTPConfig{Host: "h", Port: 587}},
		{"bad from", mail.SMTPConfig{Host: "h", Port: 587, From: "not an address"}},
		{"plain port 25", mail.SMTPConfig{Host: "h", Port: 25, From: "a@b.test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := mail.NewSMTP(tc.cfg); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// The message is composed as an email: the right headers, the address it was
// told to use, and the code in the body.
func TestSMTPSendsTheMessage(t *testing.T) {
	f := &fakeSMTP{}
	host, port := start(t, f)
	m, err := mail.NewSMTP(mail.SMTPConfig{
		Host: host, Port: 587, Username: "u", Password: "p",
		From: "hello@cuckoo.test", FromName: "Cuckoo",
	})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	mail.SetAddrForTesting(m, net.JoinHostPort(host, strconv.Itoa(port)))
	mail.TrustAnythingForTesting(m)

	err = m.Send(context.Background(), mail.Message{
		To: "priya@example.test", Subject: "Your Cuckoo sign-in code",
		Text: "Your sign-in code is 482913.\n.a line that starts with a dot",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	body, from, to, authed := f.body()
	if !authed {
		t.Error("did not authenticate")
	}
	if !strings.Contains(from, "hello@cuckoo.test") || !strings.Contains(to, "priya@example.test") {
		t.Errorf("envelope: from %q to %q", from, to)
	}
	for _, want := range []string{
		"From: Cuckoo <hello@cuckoo.test>",
		"To: priya@example.test",
		"Subject: Your Cuckoo sign-in code",
		"Content-Type: text/plain; charset=utf-8",
		"482913",
		// The line beginning with a dot did not end the message early.
		"a line that starts with a dot",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("message does not contain %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "http://") || strings.Contains(body, "https://") {
		t.Error("the message carries a link; codes are typed, not clicked")
	}
}

// A provider that will not offer TLS is refused: a sign-in code in the clear
// is a sign-in code anyone on the path can read.
func TestSMTPRefusesToSendWithoutTLS(t *testing.T) {
	f := &fakeSMTP{noSTARTTLS: true}
	host, port := start(t, f)
	m, err := mail.NewSMTP(mail.SMTPConfig{Host: host, Port: 587, From: "a@b.test"})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	mail.SetAddrForTesting(m, net.JoinHostPort(host, strconv.Itoa(port)))
	err = m.Send(context.Background(), mail.Message{To: "c@d.test", Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("err = %v, want a refusal naming STARTTLS", err)
	}
	if body, _, _, _ := f.body(); body != "" {
		t.Error("it sent the message anyway")
	}
}

// A provider that accepts the connection and then says nothing must not hold
// a person's sign-in open.
func TestSMTPGivesUp(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		time.Sleep(5 * time.Second)
		_ = conn.Close()
	}()
	addr := ln.Addr().(*net.TCPAddr)
	m, err := mail.NewSMTP(mail.SMTPConfig{Host: "127.0.0.1", Port: 587, From: "a@b.test"})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	mail.SetAddrForTesting(m, addr.String())
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := m.Send(ctx, mail.Message{To: "c@d.test", Subject: "s", Text: "t"}); err == nil {
		t.Error("a silent server did not fail")
	} else if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") &&
		!strings.Contains(err.Error(), "timeout") {
		t.Logf("gave up with: %v", err)
	}
}
