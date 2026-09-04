package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/api"
	"github.com/Saieshwar5/cuckoo/server/internal/auth"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// Server is the whole HTTP stack — routing, middleware, authentication,
// handlers — over a transactional store.
//
// Tests exercise the real router rather than calling handlers directly, so
// middleware, status codes and the error envelope are covered by every test
// that touches an endpoint, not by separate tests that could drift.
type Server struct {
	*httptest.Server
	Store *store.Store
}

// NewServer builds the API against the given store.
func NewServer(t *testing.T, db *store.Store) *Server {
	t.Helper()

	handler := api.NewRouter(api.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:   auth.NewDev(),
		Users:  users.New(db),
		Health: map[string]api.HealthCheck{},
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Server{Server: srv, Store: db}
}

// Client issues requests as one caller.
type Client struct {
	t       *testing.T
	baseURL string
	headers map[string]string
}

// AsUser returns a client authenticated as the given user.
func (s *Server) AsUser(t *testing.T, user users.User) *Client {
	t.Helper()
	return &Client{
		t:       t,
		baseURL: s.URL,
		headers: map[string]string{auth.DevHeader: domain.FormatID(domain.PrefixUser, user.ID)},
	}
}

// Anonymous returns a client with no credentials.
func (s *Server) Anonymous(t *testing.T) *Client {
	t.Helper()
	return &Client{t: t, baseURL: s.URL, headers: map[string]string{}}
}

// WithHeader returns a copy of the client with an extra header, for testing
// malformed or unexpected credentials.
func (c *Client) WithHeader(key, value string) *Client {
	headers := make(map[string]string, len(c.headers)+1)
	for k, v := range c.headers {
		headers[k] = v
	}
	headers[key] = value
	return &Client{t: c.t, baseURL: c.baseURL, headers: headers}
}

// Get issues a GET request.
func (c *Client) Get(path string) *Response { return c.do(http.MethodGet, path, nil) }

// Patch issues a PATCH request with a JSON body.
func (c *Client) Patch(path string, body any) *Response {
	return c.do(http.MethodPatch, path, body)
}

// PatchRaw issues a PATCH request with a body sent exactly as given, for
// testing malformed JSON.
func (c *Client) PatchRaw(path, body string) *Response {
	return c.send(http.MethodPatch, path, []byte(body), "application/json")
}

func (c *Client) do(method, path string, body any) *Response {
	c.t.Helper()

	var encoded []byte
	contentType := ""
	if body != nil {
		var err error
		if encoded, err = json.Marshal(body); err != nil {
			c.t.Fatalf("testutil: encode request body: %v", err)
		}
		contentType = "application/json"
	}
	return c.send(method, path, encoded, contentType)
}

func (c *Client) send(method, path string, body []byte, contentType string) *Response {
	c.t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		c.t.Fatalf("testutil: build request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("testutil: %s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("testutil: read response body: %v", err)
	}

	return &Response{t: c.t, Status: resp.StatusCode, Body: payload}
}
