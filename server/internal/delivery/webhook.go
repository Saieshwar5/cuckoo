package delivery

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
)

// Headers on every webhook request. The signature covers the timestamp and
// the raw body, so a backend can refuse both tampering and replay.
const (
	HeaderEvent     = "X-Cuckoo-Event"
	HeaderEventID   = "X-Cuckoo-Event-Id"
	HeaderTimestamp = "X-Cuckoo-Timestamp"
	HeaderSignature = "X-Cuckoo-Signature"
)

// webhookTimeout is how long a backend has to return 2xx. The body is the
// acknowledgement, not the reply; real work happens after it responds.
const webhookTimeout = 10 * time.Second

// Sign computes the value of the signature header: hex HMAC-SHA256 over
// "<timestamp>.<body>" with the endpoint's signing key, prefixed "sha256=".
// Exported so a test, or a Go SDK, verifies with the same code.
func Sign(key []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// webhookSender posts events to backends over HTTP.
type webhookSender struct {
	client *http.Client
}

func newWebhookSender(allowLoopback bool) *webhookSender {
	dialer := &safeDialer{
		allowLoopback: allowLoopback,
		dialer:        &net.Dialer{Timeout: 5 * time.Second},
	}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
	return &webhookSender{client: &http.Client{
		Transport: transport,
		Timeout:   webhookTimeout,
		// A redirect would be followed to an address we never checked. A 3xx
		// is simply not a 2xx, and is retried like any other failure.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// Send posts one event and reports whether the backend acknowledged it.
func (s *webhookSender) Send(ctx context.Context, ep *agents.Endpoint, env events.Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cuckoo-hub/1")
	req.Header.Set(HeaderEvent, env.Type)
	req.Header.Set(HeaderEventID, env.ID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderSignature, Sign(ep.SigningKey, timestamp, body))

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Read a little so the connection can be reused, and no more, so a
	// backend cannot make us buffer whatever it likes.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("backend responded %d", resp.StatusCode)
	}
	return nil
}

// safeDialer refuses to connect to addresses inside our own network.
//
// A webhook URL is chosen by whoever set up the binding, and the hub posts
// users' messages to it. Without this, an address like a cloud metadata
// service or a database on the private network could be reached by anyone
// with an account. Hostnames are resolved here and the connection is made to
// the checked address, so a name cannot change its answer between the check
// and the dial.
type safeDialer struct {
	allowLoopback bool
	dialer        *net.Dialer
}

func (d *safeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, r := range resolved {
			ips = append(ips, r.IP)
		}
	}

	var lastErr error
	for _, ip := range ips {
		if !d.permitted(ip) {
			lastErr = fmt.Errorf("address %s of %s is not reachable from the hub", ip, host)
			continue
		}
		conn, err := d.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no address")
	}
	return nil, lastErr
}

// permitted reports whether an address is on the public internet. Loopback is
// allowed only in development, where a backend is a script on this machine.
func (d *safeDialer) permitted(ip net.IP) bool {
	if ip.IsLoopback() {
		return d.allowLoopback
	}
	if ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	// Carrier-grade NAT space (100.64.0.0/10) is private in practice and not
	// covered by IsPrivate.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 {
		return false
	}
	return true
}
