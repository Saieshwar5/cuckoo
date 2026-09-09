package authapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type verifiedJSON struct {
	Token   string `json:"token"`
	Session struct {
		ID        string `json:"id"`
		ExpiresAt string `json:"expires_at"`
	} `json:"session"`
	User struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"user"`
	IsNew bool `json:"is_new"`
}

func signIn(t *testing.T, srv *testutil.Server, email string) verifiedJSON {
	t.Helper()
	anon := srv.Anonymous(t)
	anon.Post("/v1/auth/email/start", map[string]any{"email": email}).ExpectStatus(http.StatusNoContent)
	var v verifiedJSON
	anon.Post("/v1/auth/email/verify", map[string]any{
		"email": email, "code": srv.LastCode(t, strings.ToLower(email)), "device_name": "Pixel 7",
	}).ExpectStatus(http.StatusOK).Decode(&v)
	return v
}

// Start, verify, use the token, sign out: the app's whole relationship with
// authentication.
func TestSignInFlow(t *testing.T) {
	srv := testutil.NewServer(t, testutil.NewStore(t))
	v := signIn(t, srv, "Priya@Example.com")

	if !v.IsNew || !strings.HasPrefix(v.Token, "ses_tok_") || !strings.HasPrefix(v.Session.ID, "ses_") ||
		v.User.DisplayName != "priya" || !strings.HasPrefix(v.User.ID, "usr_") {
		t.Fatalf("verified = %+v", v)
	}

	me := srv.AsSession(t, v.Token)
	var profile struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	me.Get("/v1/client/me").ExpectStatus(http.StatusOK).Decode(&profile)
	if profile.User.ID != v.User.ID {
		t.Errorf("session token acts as %s, want %s", profile.User.ID, v.User.ID)
	}
	me.Get("/v1/client/conversations").ExpectStatus(http.StatusOK)

	me.Post("/v1/auth/logout", nil).ExpectStatus(http.StatusNoContent)
	me.Get("/v1/client/me").ExpectError(http.StatusUnauthorized, "invalid_credentials")
}

// A phone and a laptop are two devices of one person, not a contradiction.
func TestSecondDeviceLeavesTheFirstSignedIn(t *testing.T) {
	srv := testutil.NewServer(t, testutil.NewStore(t))
	first := signIn(t, srv, "priya@example.com")
	second := signIn(t, srv, "priya@example.com")

	if second.IsNew || second.User.ID != first.User.ID {
		t.Errorf("second sign-in = %+v, want the same account", second)
	}
	srv.AsSession(t, first.Token).Get("/v1/client/me").ExpectStatus(http.StatusOK)
	srv.AsSession(t, second.Token).Get("/v1/client/me").ExpectStatus(http.StatusOK)
}

func TestSignInRejections(t *testing.T) {
	srv := testutil.NewServer(t, testutil.NewStore(t))
	anon := srv.Anonymous(t)

	t.Run("bad email", func(t *testing.T) {
		resp := anon.Post("/v1/auth/email/start", map[string]any{"email": "nope"}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_email")
		if resp.ErrorField() != "email" {
			t.Errorf("field = %q", resp.ErrorField())
		}
	})
	t.Run("wrong code", func(t *testing.T) {
		anon.Post("/v1/auth/email/start", map[string]any{"email": "priya@example.com"}).ExpectStatus(http.StatusNoContent)
		anon.Post("/v1/auth/email/verify", map[string]any{"email": "priya@example.com", "code": "000000"}).
			ExpectError(http.StatusUnauthorized, "invalid_code")
	})
	t.Run("no code requested", func(t *testing.T) {
		anon.Post("/v1/auth/email/verify", map[string]any{"email": "nobody@example.com", "code": "123456"}).
			ExpectError(http.StatusUnauthorized, "invalid_code")
	})
	t.Run("unknown field", func(t *testing.T) {
		anon.Post("/v1/auth/email/start", map[string]any{"email": "a@b.com", "password": "x"}).
			ExpectError(http.StatusUnprocessableEntity, "unknown_field")
	})
	t.Run("too many codes", func(t *testing.T) {
		for range 5 {
			anon.Post("/v1/auth/email/start", map[string]any{"email": "eager@example.com"}).ExpectStatus(http.StatusNoContent)
		}
		resp := anon.Post("/v1/auth/email/start", map[string]any{"email": "eager@example.com"}).
			ExpectError(http.StatusTooManyRequests, "too_many_codes")
		if resp.Header.Get("Retry-After") == "" {
			t.Error("no Retry-After on a rate-limited start")
		}
	})
	t.Run("garbage token", func(t *testing.T) {
		srv.AsSession(t, "ses_tok_garbage").Get("/v1/client/me").ExpectError(http.StatusUnauthorized, "invalid_credentials")
		srv.AsSession(t, "").Get("/v1/client/me").ExpectError(http.StatusUnauthorized, "missing_credentials")
	})
	t.Run("logout with the dev header", func(t *testing.T) {
		user := testutil.CreateUser(t, srv.Store)
		srv.AsUser(t, user).Post("/v1/auth/logout", nil).ExpectError(http.StatusUnprocessableEntity, "no_session")
	})
	t.Run("logout anonymous", func(t *testing.T) {
		anon.Post("/v1/auth/logout", nil).ExpectStatus(http.StatusUnauthorized)
	})
}
