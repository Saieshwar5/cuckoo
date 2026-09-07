package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api"
	"github.com/Saieshwar5/cuckoo/server/internal/apikeys"
	"github.com/Saieshwar5/cuckoo/server/internal/auth"
	"github.com/Saieshwar5/cuckoo/server/internal/blobs"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/mail"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/retention"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
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
	// Conversations is the service the server itself uses, wired to its
	// live-update bus, for tests that need to drive it from outside a
	// request — running the delivery worker, say.
	Conversations *conversations.Service
	// Pairing is the service behind the pair and contact routes.
	Pairing *pairing.Service
	// Media is the service behind uploads, downloads and faces.
	Media *media.Service
	// Mail holds every email the server "sent", so a test can read a
	// sign-in code back.
	Mail *mail.Memory
}

// NewServer builds the API against the given store.
// ServerOption tunes the test server.
type ServerOption func(*serverConfig)

type serverConfig struct {
	welcomeHandle string
	now           func() time.Time
}

// WithWelcomeHandle names the agent every new account is given, as
// CUCKOO_WELCOME_HANDLE would.
func WithWelcomeHandle(handle string) ServerOption {
	return func(c *serverConfig) { c.welcomeHandle = handle }
}

// WithClock moves the server's idea of now, which is what retention measures
// the window from. A test can make a conversation old without waiting.
func WithClock(now func() time.Time) ServerOption {
	return func(c *serverConfig) { c.now = now }
}

// TestRetention is the window the test server applies: the production
// defaults, so what a test sees is what a person sees.
var TestRetention = retention.Policy{
	MessageAge:       90 * 24 * time.Hour,
	UserMediaBudget:  100 << 20,
	AgentMediaBudget: 1024 << 20,
}

func NewServer(t *testing.T, db *store.Store, opts ...ServerOption) *Server {
	t.Helper()
	var sc serverConfig
	for _, o := range opts {
		o(&sc)
	}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Live updates go through a real Redis channel of this test's own, so a
	// socket test proves the whole path, publish to frame.
	bus := NewBus(t)
	hub := realtime.NewHub(quiet)
	ctx, cancel := context.WithCancel(context.Background())
	if err := hub.Start(ctx, bus); err != nil {
		cancel()
		t.Fatalf("testutil: start realtime hub: %v", err)
	}
	t.Cleanup(cancel)
	// Socket handlers are hijacked connections, which the HTTP server's
	// Close does not wait for. Waiting here keeps a handler's last database
	// call inside this test's transaction rather than the next test's.
	t.Cleanup(func() {
		hub.Close()
		wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer wcancel()
		if err := hub.Wait(wctx); err != nil {
			t.Errorf("testutil: socket handlers still running at cleanup: %v", err)
		}
	})

	mailer := mail.NewMemory()

	agentService := agents.New(db, agents.WithPublisher(bus))
	conversationService := conversations.New(db,
		conversations.WithLimiter(NewLimiter(t)),
		conversations.WithPublisher(bus),
		conversations.WithStreams(NewStreamStore(t)))
	userService := users.New(db)
	pairingService := pairing.New(db, agentService, conversationService, userService, "https://hub.test")

	signinService := signin.New(db, mailer, signin.WithLimiter(NewLimiter(t)),
		signin.WithWelcome(pairingService.Welcomer(sc.welcomeHandle)))
	apiKeyService := apikeys.New(db)
	// Uploads land in a directory the test framework removes with the test.
	blobStore, err := blobs.NewDisk(t.TempDir())
	if err != nil {
		t.Fatalf("testutil: media storage: %v", err)
	}
	mediaService := media.New(db, blobStore, media.WithLogger(quiet))
	retentionOpts := []retention.Option{retention.WithLogger(quiet)}
	if sc.now != nil {
		retentionOpts = append(retentionOpts, retention.WithClock(sc.now))
	}
	retentionService := retention.New(db, blobStore, TestRetention, retentionOpts...)
	userAuth := auth.NewDevOrSession(auth.NewDev(), auth.NewSession(signinService))
	handler := api.NewRouter(api.Deps{
		Logger:        quiet,
		UserAuth:      userAuth,
		AgentAuth:     auth.NewBinding(agentService),
		MgmtAuth:      auth.NewKeyOrUser(apiKeyService, userAuth),
		Users:         userService,
		SignIn:        signinService,
		Agents:        agentService,
		Conversations: conversationService,
		Delivery:      delivery.New(db, conversationService),
		Pairing:       pairingService,
		APIKeys:       apiKeyService,
		Media:         mediaService,
		PublicURL:     "https://hub.test",
		Hub:           hub,
		Bus:           bus,
		CORSOrigins:   []string{"*"},
		Retention:     retentionService,
		Health:        map[string]api.HealthCheck{},
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Server{
		Server: srv, Store: db, Conversations: conversationService,
		Pairing: pairingService, Media: mediaService, Mail: mailer,
	}
}

// AsKey returns a client authenticated with an API key, as a company's own
// server is.
func (s *Server) AsKey(t *testing.T, key string) *Client {
	t.Helper()
	return s.AsSession(t, key)
}

// AsSession returns a client authenticated with a session token, as the app
// is once signed in.
func (s *Server) AsSession(t *testing.T, token string) *Client {
	t.Helper()
	return &Client{
		t:       t,
		baseURL: s.URL,
		headers: map[string]string{"Authorization": "Bearer " + token},
	}
}

// SignIn signs a person in through the real endpoints and returns their
// session token.
func (s *Server) SignIn(t *testing.T, email string) string {
	t.Helper()
	anon := s.Anonymous(t)
	anon.Post("/v1/auth/email/start", map[string]any{"email": email}).ExpectStatus(http.StatusNoContent)
	var v struct {
		Token string `json:"token"`
	}
	anon.Post("/v1/auth/email/verify", map[string]any{"email": email, "code": s.LastCode(t, strings.ToLower(email))}).
		ExpectStatus(http.StatusOK).Decode(&v)
	return v.Token
}

// LastCode reads the sign-in code most recently mailed to an address.
func (s *Server) LastCode(t *testing.T, email string) string {
	t.Helper()
	m, ok := s.Mail.Last(email)
	if !ok {
		t.Fatalf("testutil: no mail was sent to %s", email)
	}
	code := codePattern.FindString(m.Text)
	if code == "" {
		t.Fatalf("testutil: no code in mail to %s: %q", email, m.Text)
	}
	return code
}

var codePattern = regexp.MustCompile(`\b[0-9]{6}\b`)

// Socket opens the app's live-update socket as the given user. The
// connection is closed when the test ends.
func (s *Server) Socket(t *testing.T, user users.User) *websocket.Conn {
	t.Helper()
	c, err := s.DialSocket(t, "/v1/client/socket",
		http.Header{auth.DevHeader: {domain.FormatID(domain.PrefixUser, user.ID)}})
	if err != nil {
		t.Fatalf("testutil: open socket: %v", err)
	}
	return c
}

// AgentSocket opens the agent protocol's socket with a binding secret, as a
// backend would.
func (s *Server) AgentSocket(t *testing.T, secret string) *websocket.Conn {
	t.Helper()
	c, err := s.DialSocket(t, "/v1/agent/socket", http.Header{"Authorization": {"Bearer " + secret}})
	if err != nil {
		t.Fatalf("testutil: open agent socket: %v", err)
	}
	return c
}

// DialSocket opens a socket at path with the given headers, returning the
// handshake error so a test can assert on a refused connection.
func (s *Server) DialSocket(t *testing.T, path string, headers http.Header) (*websocket.Conn, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(s.URL, "http") + path
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: headers})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("%w (status %d)", err, resp.StatusCode)
		}
		return nil, err
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c, nil
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

// AsAgent returns a client authenticated with a binding secret, as an agent
// backend would be.
func (s *Server) AsAgent(t *testing.T, secret string) *Client {
	t.Helper()
	return &Client{
		t:       t,
		baseURL: s.URL,
		headers: map[string]string{"Authorization": "Bearer " + secret},
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

// Post issues a POST request with a JSON body.
func (c *Client) Post(path string, body any) *Response {
	return c.do(http.MethodPost, path, body)
}

// Patch issues a PATCH request with a JSON body.
func (c *Client) Patch(path string, body any) *Response {
	return c.do(http.MethodPatch, path, body)
}

// Delete issues a DELETE request.
func (c *Client) Delete(path string) *Response { return c.do(http.MethodDelete, path, nil) }

// Options issues an OPTIONS request, as a browser's preflight does.
func (c *Client) Options(path string) *Response { return c.do(http.MethodOptions, path, nil) }

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

	return &Response{t: c.t, Status: resp.StatusCode, Header: resp.Header, Body: payload}
}

// Upload posts a file as a multipart form, the way a phone does.
func (c *Client) Upload(path, fileName string, data []byte) *Response {
	c.t.Helper()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", fileName)
	if err != nil {
		c.t.Fatalf("testutil: build upload: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		c.t.Fatalf("testutil: write upload: %v", err)
	}
	if err := form.Close(); err != nil {
		c.t.Fatalf("testutil: finish upload: %v", err)
	}
	return c.send(http.MethodPost, path, body.Bytes(), form.FormDataContentType())
}

// UploadRaw posts a file as the request body, the way one line of curl does.
func (c *Client) UploadRaw(path string, data []byte) *Response {
	c.t.Helper()
	return c.send(http.MethodPost, path, data, "application/octet-stream")
}
