package delivery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
)

func TestSign(t *testing.T) {
	key := domain.HashSecret("bnd_sec_example")
	sig := Sign(key, "1725436800", []byte(`{"a":1}`))

	if !strings.HasPrefix(sig, "sha256=") || len(sig) != len("sha256=")+64 {
		t.Fatalf("signature has the wrong shape: %q", sig)
	}
	if Sign(key, "1725436800", []byte(`{"a":1}`)) != sig {
		t.Error("signature is not deterministic")
	}
	if Sign(key, "1725436801", []byte(`{"a":1}`)) == sig {
		t.Error("timestamp is not covered by the signature")
	}
	if Sign(key, "1725436800", []byte(`{"a":2}`)) == sig {
		t.Error("body is not covered by the signature")
	}
	if Sign(domain.HashSecret("bnd_sec_other"), "1725436800", []byte(`{"a":1}`)) == sig {
		t.Error("key is not covered by the signature")
	}
}

func TestSafeDialerPermitted(t *testing.T) {
	blocked := []string{
		"10.0.0.1", "172.16.0.1", "192.168.1.1", // private
		"169.254.169.254",      // link-local, where cloud metadata lives
		"100.64.0.1",           // carrier-grade NAT
		"0.0.0.0", "224.0.0.1", // unspecified, multicast
		"fc00::1", "fe80::1", "::", "ff02::1",
	}
	public := []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"}

	strict := &safeDialer{allowLoopback: false}
	for _, s := range blocked {
		if strict.permitted(net.ParseIP(s)) {
			t.Errorf("%s permitted, want blocked", s)
		}
	}
	for _, s := range public {
		if !strict.permitted(net.ParseIP(s)) {
			t.Errorf("%s blocked, want permitted", s)
		}
	}

	for _, s := range []string{"127.0.0.1", "::1"} {
		if strict.permitted(net.ParseIP(s)) {
			t.Errorf("%s permitted in production", s)
		}
		if !(&safeDialer{allowLoopback: true}).permitted(net.ParseIP(s)) {
			t.Errorf("%s blocked in development", s)
		}
	}
}

// The guard has to be wired into the client that actually sends, not just
// exist: a loopback backend is refused unless loopback is allowed.
func TestSendRefusesAddressesInsideTheNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	ep := &agents.Endpoint{Mode: agents.ModeWebhook, WebhookURL: srv.URL, SigningKey: make([]byte, 32)}

	err := newWebhookSender(false).Send(context.Background(), ep, events.Envelope{})
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("production sender reached loopback: %v", err)
	}
	if err := newWebhookSender(true).Send(context.Background(), ep, events.Envelope{}); err != nil {
		t.Errorf("development sender refused loopback: %v", err)
	}
}

// A redirect is not followed: the target was never checked, and a 3xx is not
// an acknowledgement.
func TestSendDoesNotFollowRedirects(t *testing.T) {
	var followed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			followed = true
			return
		}
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer srv.Close()
	ep := &agents.Endpoint{Mode: agents.ModeWebhook, WebhookURL: srv.URL, SigningKey: make([]byte, 32)}

	err := newWebhookSender(true).Send(context.Background(), ep, events.Envelope{})
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Errorf("redirect was treated as success: %v", err)
	}
	if followed {
		t.Error("redirect was followed")
	}
}
