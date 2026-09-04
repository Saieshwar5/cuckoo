package agents_test

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func TestSetBindingSocket(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "sock", DisplayName: "S"})

	binding, secret, err := svc.SetBinding(ctx, f.owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket})
	if err != nil {
		t.Fatalf("SetBinding: %v", err)
	}
	if binding.Mode != agents.ModeSocket || binding.WebhookURL != nil {
		t.Errorf("binding = %+v, want socket with no URL", binding)
	}
	if binding.Status != agents.StatusIdle {
		t.Errorf("new binding Status = %q, want idle", binding.Status)
	}
	if !strings.HasPrefix(secret, "bnd_sec_") {
		t.Errorf("secret %q lacks the bnd_sec_ prefix", secret)
	}

	got, err := svc.ActiveBinding(ctx, f.owner.ID, agent.ID)
	if err != nil || got == nil || got.ID != binding.ID {
		t.Errorf("ActiveBinding = %+v, %v; want the binding just set", got, err)
	}
}

// The plaintext secret must exist nowhere but in the response that created
// it. The database holds only its SHA-256.
func TestSecretIsStoredOnlyAsHash(t *testing.T) {
	ctx := context.Background()
	db, tx := testutil.NewStoreTx(t)
	svc := agents.New(db)
	owner := testutil.CreateUser(t, db)
	agent, _ := svc.Create(ctx, owner.ID, agents.CreateInput{Handle: "hashed", DisplayName: "H"})

	_, secret, err := svc.SetBinding(ctx, owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket})
	if err != nil {
		t.Fatalf("SetBinding: %v", err)
	}

	var stored []byte
	if err := tx.QueryRow(ctx, "SELECT secret_hash FROM agent_bindings WHERE agent_id = $1", agent.ID).Scan(&stored); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	want := sha256.Sum256([]byte(secret))
	if string(stored) != string(want[:]) {
		t.Error("stored value is not SHA-256 of the secret")
	}
	if strings.Contains(string(stored), secret) {
		t.Error("the plaintext secret is in the database")
	}
}

func TestSetBindingWebhook(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "hook", DisplayName: "W"})

	binding, _, err := svc.SetBinding(ctx, f.owner.ID, agent.ID, agents.SetBindingInput{
		Mode: agents.ModeWebhook, WebhookURL: "https://example.com/cuckoo",
	})
	if err != nil {
		t.Fatalf("SetBinding: %v", err)
	}
	if binding.WebhookURL == nil || *binding.WebhookURL != "https://example.com/cuckoo" {
		t.Errorf("WebhookURL = %v", binding.WebhookURL)
	}
}

func TestSetBindingRejectsMismatchedInput(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "mismatch", DisplayName: "M"})

	cases := map[string]agents.SetBindingInput{
		"webhook without url": {Mode: agents.ModeWebhook},
		"webhook plain http":  {Mode: agents.ModeWebhook, WebhookURL: "http://example.com/h"},
		"socket with url":     {Mode: agents.ModeSocket, WebhookURL: "https://example.com/h"},
		"unknown mode":        {Mode: "carrier-pigeon"},
		"empty mode":          {},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := svc.SetBinding(ctx, f.owner.ID, agent.ID, in); domain.KindOf(err) != domain.KindInvalid {
				t.Errorf("got %v, want invalid", err)
			}
		})
	}
	// Nothing above may have left a binding behind.
	if b, _ := svc.ActiveBinding(ctx, f.owner.ID, agent.ID); b != nil {
		t.Errorf("a rejected SetBinding created a binding: %+v", b)
	}
}

// Setting a binding replaces the old one atomically: the old secret dies at
// the instant the new one is born, never both alive, never neither.
func TestSetBindingReplacesPrevious(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "replace", DisplayName: "R"})

	first, oldSecret, _ := svc.SetBinding(ctx, f.owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket})
	second, newSecret, err := svc.SetBinding(ctx, f.owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket})
	if err != nil {
		t.Fatalf("second SetBinding: %v", err)
	}
	if first.ID == second.ID || oldSecret == newSecret {
		t.Fatal("second SetBinding did not create a new binding")
	}

	active, _ := svc.ActiveBinding(ctx, f.owner.ID, agent.ID)
	if active == nil || active.ID != second.ID {
		t.Errorf("active binding = %+v, want the second", active)
	}
	if _, err := svc.ResolveSecret(ctx, oldSecret); domain.KindOf(err) != domain.KindUnauthorized {
		t.Errorf("old secret still resolves: %v", err)
	}
	if id, err := svc.ResolveSecret(ctx, newSecret); err != nil || id != agent.ID {
		t.Errorf("new secret resolves to %v, %v", id, err)
	}
}

func TestRevokeBinding(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "revoke", DisplayName: "V"})

	if err := svc.RevokeBinding(ctx, f.owner.ID, agent.ID); domain.CodeOf(err) != "no_binding" {
		t.Errorf("revoking nothing got %v, want no_binding", err)
	}

	_, secret, _ := svc.SetBinding(ctx, f.owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket})
	if err := svc.RevokeBinding(ctx, f.owner.ID, agent.ID); err != nil {
		t.Fatalf("RevokeBinding: %v", err)
	}

	if b, _ := svc.ActiveBinding(ctx, f.owner.ID, agent.ID); b != nil {
		t.Errorf("binding still active after revoke: %+v", b)
	}
	if _, err := svc.ResolveSecret(ctx, secret); domain.KindOf(err) != domain.KindUnauthorized {
		t.Errorf("revoked secret still resolves: %v", err)
	}
	// The agent itself is untouched — revoke is not delete.
	if _, err := svc.Get(ctx, agent.ID); err != nil {
		t.Errorf("revoking the binding affected the agent: %v", err)
	}
}

func TestBindingOperationsRequireOwner(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "guarded", DisplayName: "G"})
	svc.SetBinding(ctx, f.owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket}) //nolint:errcheck

	if _, _, err := svc.SetBinding(ctx, f.other.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket}); domain.KindOf(err) != domain.KindForbidden {
		t.Errorf("SetBinding by non-owner: %v", err)
	}
	if err := svc.RevokeBinding(ctx, f.other.ID, agent.ID); domain.KindOf(err) != domain.KindForbidden {
		t.Errorf("RevokeBinding by non-owner: %v", err)
	}
	if _, err := svc.ActiveBinding(ctx, f.other.ID, agent.ID); domain.KindOf(err) != domain.KindForbidden {
		t.Errorf("ActiveBinding by non-owner: %v", err)
	}
}

func TestResolveSecretRejectsGarbage(t *testing.T) {
	_, svc := setup(t)
	for _, s := range []string{"", "bnd_sec_", "bnd_sec_notreal", "usr_k3b7sejk5937150g9f149axbt8", "Bearer x"} {
		if _, err := svc.ResolveSecret(context.Background(), s); domain.CodeOf(err) != "invalid_credentials" {
			t.Errorf("ResolveSecret(%q) = %v, want invalid_credentials", s, err)
		}
	}
}
