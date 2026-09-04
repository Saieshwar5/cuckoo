package agents_test

import (
	"context"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

type fixture struct {
	svc   *agents.Service
	owner users.User
	other users.User
}

func setup(t *testing.T) (*fixture, *agents.Service) {
	t.Helper()
	db := testutil.NewStore(t)
	f := &fixture{
		svc:   agents.New(db),
		owner: testutil.CreateUser(t, db, testutil.WithDisplayName("Owner")),
		other: testutil.CreateUser(t, db, testutil.WithDisplayName("Someone Else")),
	}
	return f, f.svc
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)

	agent, err := svc.Create(ctx, f.owner.ID, agents.CreateInput{
		Handle: "SBI-Support", DisplayName: "SBI Support", Description: "Banking help.",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if agent.OwnerID != f.owner.ID {
		t.Errorf("OwnerID = %v, want the creator", agent.OwnerID)
	}
	if agent.Handle != "sbi-support" {
		t.Errorf("Handle = %q, want lowercased sbi-support", agent.Handle)
	}
	if agent.DisplayName != "SBI Support" || agent.Description != "Banking help." {
		t.Errorf("fields not stored: %+v", agent)
	}

	got, err := svc.Get(ctx, agent.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != agent {
		t.Errorf("Get = %+v, want %+v", got, agent)
	}
}

// Handles are unique across the whole hub, not per owner: they form the
// agent's identifier, and two agents with one identifier is not an identifier.
func TestCreateRejectsDuplicateHandleAcrossOwners(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)

	if _, err := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "helper", DisplayName: "A"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, f.other.ID, agents.CreateInput{Handle: "HELPER", DisplayName: "B"})
	if err == nil {
		t.Fatal("a second agent took the same handle")
	}
	if domain.KindOf(err) != domain.KindConflict || domain.CodeOf(err) != "handle_taken" {
		t.Errorf("got %v, want conflict/handle_taken", err)
	}
}

// A retired agent keeps its handle. Reissuing it would let a new agent
// inherit the identity people remembered the old one by.
func TestDeletedAgentHandleIsNotReissued(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)

	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "retired", DisplayName: "Old"})
	if err := svc.Delete(ctx, f.owner.ID, agent.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := svc.Create(ctx, f.other.ID, agents.CreateInput{Handle: "retired", DisplayName: "New"})
	if domain.CodeOf(err) != "handle_taken" {
		t.Errorf("a deleted agent's handle was reissued: %v", err)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	f, svc := setup(t)
	cases := map[string]agents.CreateInput{
		"bad handle":       {Handle: "no spaces", DisplayName: "X"},
		"reserved":         {Handle: "admin", DisplayName: "X"},
		"empty name":       {Handle: "ok-handle", DisplayName: ""},
		"newline name":     {Handle: "ok-handle", DisplayName: "a\nb"},
		"long description": {Handle: "ok-handle", DisplayName: "X", Description: string(make([]byte, 501))},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Create(context.Background(), f.owner.ID, in); err == nil {
				t.Fatal("accepted invalid input")
			} else if domain.KindOf(err) != domain.KindInvalid {
				t.Errorf("Kind = %q, want invalid", domain.KindOf(err))
			}
		})
	}
}

func TestGetMissing(t *testing.T) {
	_, svc := setup(t)
	_, err := svc.Get(context.Background(), domain.NewID())
	if domain.CodeOf(err) != "agent_not_found" {
		t.Errorf("got %v, want agent_not_found", err)
	}
}

// Ownership is the first authorisation rule in the system. A missing agent
// and someone else's agent are different answers on purpose.
func TestGetOwnedDistinguishesMissingFromForbidden(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "mine", DisplayName: "Mine"})

	if _, err := svc.GetOwned(ctx, f.owner.ID, agent.ID); err != nil {
		t.Errorf("owner refused: %v", err)
	}
	if _, err := svc.GetOwned(ctx, f.other.ID, agent.ID); domain.KindOf(err) != domain.KindForbidden {
		t.Errorf("non-owner got %v, want forbidden", err)
	}
	if _, err := svc.GetOwned(ctx, f.owner.ID, domain.NewID()); domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("missing agent got %v, want not_found", err)
	}
}

func TestListMineReturnsOnlyMineInOrder(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)

	a, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "first", DisplayName: "1"})
	b, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "second", DisplayName: "2"})
	svc.Create(ctx, f.other.ID, agents.CreateInput{Handle: "theirs", DisplayName: "3"}) //nolint:errcheck

	list, err := svc.ListMine(ctx, f.owner.ID)
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Errorf("ListMine = %v, want [%v %v]", list, a.ID, b.ID)
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{
		Handle: "upd", DisplayName: "Before", Description: "keep me",
	})

	name := "After"
	got, err := svc.Update(ctx, f.owner.ID, agent.ID, agents.UpdateInput{DisplayName: &name})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.DisplayName != "After" || got.Description != "keep me" {
		t.Errorf("partial update touched the wrong fields: %+v", got)
	}
	if got.Handle != "upd" {
		t.Error("Update changed the handle")
	}

	if _, err := svc.Update(ctx, f.owner.ID, agent.ID, agents.UpdateInput{}); domain.CodeOf(err) != "no_changes" {
		t.Errorf("empty update got %v, want no_changes", err)
	}
	if _, err := svc.Update(ctx, f.other.ID, agent.ID, agents.UpdateInput{DisplayName: &name}); domain.KindOf(err) != domain.KindForbidden {
		t.Errorf("non-owner update got %v, want forbidden", err)
	}
}

func TestDeleteRetiresAgentAndRevokesBinding(t *testing.T) {
	ctx := context.Background()
	f, svc := setup(t)
	agent, _ := svc.Create(ctx, f.owner.ID, agents.CreateInput{Handle: "doomed", DisplayName: "D"})
	_, secret, err := svc.SetBinding(ctx, f.owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket})
	if err != nil {
		t.Fatalf("SetBinding: %v", err)
	}

	if err := svc.Delete(ctx, f.other.ID, agent.ID); domain.KindOf(err) != domain.KindForbidden {
		t.Errorf("non-owner delete got %v, want forbidden", err)
	}
	if err := svc.Delete(ctx, f.owner.ID, agent.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := svc.Get(ctx, agent.ID); domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("deleted agent still readable: %v", err)
	}
	// The binding must die with the agent, in the same transaction.
	if _, err := svc.ResolveSecret(ctx, secret); domain.KindOf(err) != domain.KindUnauthorized {
		t.Errorf("a deleted agent's secret still authenticates: %v", err)
	}
	if err := svc.Delete(ctx, f.owner.ID, agent.ID); domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("second delete got %v, want not_found", err)
	}
}
