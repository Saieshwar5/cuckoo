package client_test

import (
	"net/http"
	"strings"
	"testing"
)

// A browser-hosted app calls the hub from another origin; the preflight and
// the real request both need the headers, or the browser refuses.
func TestCORSForBrowsers(t *testing.T) {
	f := setupChat(t)

	pre := f.srv.Anonymous(t).
		WithHeader("Origin", "http://localhost:8081").
		WithHeader("Access-Control-Request-Method", "POST").
		Options("/v1/auth/email/start")
	if pre.Status != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", pre.Status)
	}
	if pre.Header.Get("Access-Control-Allow-Origin") != "http://localhost:8081" ||
		!strings.Contains(pre.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("preflight headers = %v", pre.Header)
	}

	resp := f.srv.AsUser(t, f.owner).WithHeader("Origin", "http://localhost:8081").Get("/v1/client/me")
	if resp.Status != http.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:8081" {
		t.Errorf("real request: status %d, allow-origin %q", resp.Status, resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if f.srv.AsUser(t, f.owner).Get("/v1/client/me").Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("a request with no Origin got CORS headers")
	}
}
