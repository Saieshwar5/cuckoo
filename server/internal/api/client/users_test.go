package client_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/auth"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// meResponse mirrors what the app parses. It is written out here rather than
// imported so that a change to the server's response type shows up as a failing
// test instead of silently changing the contract.
type meResponse struct {
	User struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Locale      string `json:"locale"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	} `json:"user"`
}

func setup(t *testing.T) (*testutil.Server, users.User) {
	t.Helper()
	db := testutil.NewStore(t)
	user := testutil.CreateUser(t, db, testutil.WithDisplayName("Priya"), testutil.WithLocale("te-IN"))
	return testutil.NewServer(t, db), user
}

func TestGetMe(t *testing.T) {
	srv, user := setup(t)

	var got meResponse
	srv.AsUser(t, user).Get("/v1/client/me").
		ExpectStatus(http.StatusOK).
		Decode(&got)

	if want := domain.FormatID(domain.PrefixUser, user.ID); got.User.ID != want {
		t.Errorf("id = %q, want %q", got.User.ID, want)
	}
	if got.User.DisplayName != "Priya" {
		t.Errorf("display_name = %q, want Priya", got.User.DisplayName)
	}
	if got.User.Locale != "te-IN" {
		t.Errorf("locale = %q, want te-IN", got.User.Locale)
	}
	if got.User.CreatedAt == "" || got.User.UpdatedAt == "" {
		t.Error("timestamps are missing from the response")
	}
}

// Identifiers are exposed in their prefixed form, never as bare UUIDs: the
// prefix is what makes a wrong identifier fail at the edge instead of deeper in.
func TestGetMeReturnsPrefixedIdentifier(t *testing.T) {
	srv, user := setup(t)

	var got meResponse
	srv.AsUser(t, user).Get("/v1/client/me").ExpectStatus(http.StatusOK).Decode(&got)

	if !strings.HasPrefix(got.User.ID, "usr_") {
		t.Errorf("id = %q, want a usr_ prefix", got.User.ID)
	}
	if strings.Contains(got.User.ID, "-") {
		t.Errorf("id = %q leaks the raw UUID form", got.User.ID)
	}
}

func TestAuthenticationIsRequired(t *testing.T) {
	srv, user := setup(t)

	cases := []struct {
		name   string
		client *testutil.Client
		code   string
	}{
		{"no credentials", srv.Anonymous(t), "missing_credentials"},
		{"empty header", srv.Anonymous(t).WithHeader(auth.DevHeader, ""), "missing_credentials"},
		{"not an identifier", srv.Anonymous(t).WithHeader(auth.DevHeader, "hello"), "invalid_credentials"},
		{"bare uuid", srv.Anonymous(t).WithHeader(auth.DevHeader,
			"9b2e4a10-0000-7000-8000-000000000000"), "invalid_credentials"},
		{"wrong id type", srv.Anonymous(t).WithHeader(auth.DevHeader,
			domain.FormatID(domain.PrefixAgent, user.ID)), "invalid_credentials"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.client.Get("/v1/client/me").ExpectError(http.StatusUnauthorized, tc.code)
		})
	}
}

// A well-formed identifier for an account that does not exist must read as
// not-found, which is also the path a deleted account takes.
func TestGetMeForUnknownUser(t *testing.T) {
	srv, _ := setup(t)

	ghost := srv.Anonymous(t).WithHeader(auth.DevHeader,
		domain.FormatID(domain.PrefixUser, domain.NewID()))

	ghost.Get("/v1/client/me").ExpectError(http.StatusNotFound, "user_not_found")
}

func TestUpdateMe(t *testing.T) {
	srv, user := setup(t)
	client := srv.AsUser(t, user)

	var updated meResponse
	client.Patch("/v1/client/me", map[string]any{"display_name": "Priya Sharma"}).
		ExpectStatus(http.StatusOK).
		Decode(&updated)

	if updated.User.DisplayName != "Priya Sharma" {
		t.Errorf("display_name = %q, want Priya Sharma", updated.User.DisplayName)
	}
	if updated.User.Locale != "te-IN" {
		t.Errorf("locale changed to %q when only the name was sent", updated.User.Locale)
	}

	// The change must be persisted, not merely reflected back.
	var reread meResponse
	client.Get("/v1/client/me").ExpectStatus(http.StatusOK).Decode(&reread)
	if reread.User.DisplayName != "Priya Sharma" {
		t.Errorf("after re-reading, display_name = %q", reread.User.DisplayName)
	}
}

func TestUpdateMeTrimsWhitespace(t *testing.T) {
	srv, user := setup(t)

	var updated meResponse
	srv.AsUser(t, user).Patch("/v1/client/me", map[string]any{"display_name": "  Ravi  "}).
		ExpectStatus(http.StatusOK).
		Decode(&updated)

	if updated.User.DisplayName != "Ravi" {
		t.Errorf("display_name = %q, want the trimmed value", updated.User.DisplayName)
	}
}

func TestUpdateMeRejectsBadRequests(t *testing.T) {
	srv, user := setup(t)
	client := srv.AsUser(t, user)

	t.Run("blank name", func(t *testing.T) {
		resp := client.Patch("/v1/client/me", map[string]any{"display_name": "   "}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_display_name")
		if field := resp.ErrorField(); field != "display_name" {
			t.Errorf("error field = %q, want display_name", field)
		}
	})

	t.Run("long name", func(t *testing.T) {
		client.Patch("/v1/client/me", map[string]any{"display_name": strings.Repeat("a", 81)}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_display_name")
	})

	t.Run("bad locale", func(t *testing.T) {
		client.Patch("/v1/client/me", map[string]any{"locale": "english"}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_locale")
	})

	t.Run("nothing to change", func(t *testing.T) {
		client.Patch("/v1/client/me", map[string]any{}).
			ExpectError(http.StatusUnprocessableEntity, "no_changes")
	})

	t.Run("unknown field", func(t *testing.T) {
		resp := client.Patch("/v1/client/me", map[string]any{"nickname": "P"}).
			ExpectError(http.StatusUnprocessableEntity, "unknown_field")
		if field := resp.ErrorField(); field != "nickname" {
			t.Errorf("error field = %q, want nickname", field)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		client.PatchRaw("/v1/client/me", `{"display_name":`).
			ExpectError(http.StatusUnprocessableEntity, "invalid_body")
	})

	t.Run("wrong type", func(t *testing.T) {
		client.PatchRaw("/v1/client/me", `{"display_name": 42}`).
			ExpectError(http.StatusUnprocessableEntity, "invalid_body")
	})
}

// A rejected update must leave the account exactly as it was.
func TestRejectedUpdateChangesNothing(t *testing.T) {
	srv, user := setup(t)
	client := srv.AsUser(t, user)

	client.Patch("/v1/client/me", map[string]any{"display_name": "", "locale": "english"}).
		ExpectStatus(http.StatusUnprocessableEntity)

	var after meResponse
	client.Get("/v1/client/me").ExpectStatus(http.StatusOK).Decode(&after)

	if after.User.DisplayName != "Priya" || after.User.Locale != "te-IN" {
		t.Errorf("a rejected update modified the account: %+v", after.User)
	}
}

func TestRoutingErrors(t *testing.T) {
	srv, user := setup(t)
	client := srv.AsUser(t, user)

	t.Run("unknown route", func(t *testing.T) {
		client.Get("/v1/client/nothing-here").
			ExpectError(http.StatusNotFound, "route_not_found")
	})

	t.Run("unknown top level route", func(t *testing.T) {
		client.Get("/nope").ExpectError(http.StatusNotFound, "route_not_found")
	})
}
