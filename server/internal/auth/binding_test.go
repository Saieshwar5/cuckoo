package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/auth"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func TestBindingAuthenticator(t *testing.T) {
	db := testutil.NewStore(t)
	owner := testutil.CreateUser(t, db)
	agent := testutil.CreateAgent(t, db, owner)
	_, secret := testutil.BindAgent(t, db, agent)

	a := auth.NewBinding(agents.New(db))

	request := func(header string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		return r
	}

	t.Run("valid", func(t *testing.T) {
		p, err := a.Authenticate(request("Bearer " + secret))
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if !p.IsAgent() || p.AgentID != agent.ID {
			t.Errorf("principal = %+v, want agent %v", p, agent.ID)
		}
	})

	t.Run("scheme is case-insensitive", func(t *testing.T) {
		if _, err := a.Authenticate(request("bearer " + secret)); err != nil {
			t.Errorf("lowercase bearer rejected: %v", err)
		}
	})

	rejected := map[string]struct{ header, code string }{
		"missing":      {"", "missing_credentials"},
		"no scheme":    {secret, "invalid_credentials"},
		"basic":        {"Basic " + secret, "invalid_credentials"},
		"empty token":  {"Bearer ", "invalid_credentials"},
		"wrong secret": {"Bearer bnd_sec_0000000000000000000000000000000000000000000000000000", "invalid_credentials"},
		"user id":      {"Bearer " + domain.FormatID(domain.PrefixUser, owner.ID), "invalid_credentials"},
	}
	for name, tc := range rejected {
		t.Run(name, func(t *testing.T) {
			_, err := a.Authenticate(request(tc.header))
			if err == nil {
				t.Fatal("accepted")
			}
			if domain.CodeOf(err) != tc.code {
				t.Errorf("code = %q, want %q", domain.CodeOf(err), tc.code)
			}
		})
	}
}
