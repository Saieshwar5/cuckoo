package conversations_test

import (
	"context"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

type fixture struct {
	db    *store.Store
	svc   *conversations.Service
	owner users.User
	other users.User
	agent agents.Agent
	dm    conversations.Conversation
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db := testutil.NewStore(t)
	f := &fixture{
		db:    db,
		svc:   conversations.New(db),
		owner: testutil.CreateUser(t, db, testutil.WithDisplayName("Priya")),
		other: testutil.CreateUser(t, db, testutil.WithDisplayName("Someone Else")),
	}
	f.agent = testutil.CreateAgent(t, db, f.owner, testutil.WithHandle("helper"), testutil.WithAgentName("Helper"))
	f.dm = testutil.OwnerDM(t, db, f.agent)
	return f
}

// Creating an agent is what opens a conversation today, so the first thing
// to prove is that an owner finds their new agent in the chat list, with both
// parties present and correctly named.
func TestAgentCreationOpensOwnerDM(t *testing.T) {
	f := setup(t)

	list, err := f.svc.ListMine(context.Background(), f.owner.ID)
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("owner has %d conversations, want 1", len(list))
	}
	dm := list[0]
	if dm.Kind != conversations.KindDM {
		t.Errorf("Kind = %q, want dm", dm.Kind)
	}
	if dm.LastMessage != nil {
		t.Errorf("a fresh DM has a last message: %+v", dm.LastMessage)
	}
	if len(dm.Participants) != 2 {
		t.Fatalf("got %d participants, want 2: %+v", len(dm.Participants), dm.Participants)
	}

	byKind := map[conversations.ParticipantKind]conversations.Participant{}
	for _, p := range dm.Participants {
		byKind[p.Kind] = p
	}
	user, agent := byKind[conversations.ParticipantUser], byKind[conversations.ParticipantAgent]
	if user.ID != f.owner.ID || user.DisplayName != "Priya" || user.Handle != "" {
		t.Errorf("user participant = %+v", user)
	}
	if agent.ID != f.agent.ID || agent.DisplayName != "Helper" || agent.Handle != "helper" {
		t.Errorf("agent participant = %+v", agent)
	}
}

func TestListMineShowsOnlyMyConversations(t *testing.T) {
	f := setup(t)
	testutil.CreateAgent(t, f.db, f.other)

	mine, _ := f.svc.ListMine(context.Background(), f.owner.ID)
	theirs, _ := f.svc.ListMine(context.Background(), f.other.ID)
	if len(mine) != 1 || mine[0].ID != f.dm.ID {
		t.Errorf("owner sees %+v, want only their own DM", mine)
	}
	if len(theirs) != 1 || theirs[0].ID == f.dm.ID {
		t.Errorf("other user sees %+v, want only their own DM", theirs)
	}
}

// The chat list is ordered by activity, like every messenger: a new
// conversation goes to the top, and so does an old one somebody just spoke in.
func TestListMineOrdersByLatestActivity(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	second := testutil.OwnerDM(t, f.db, testutil.CreateAgent(t, f.db, f.owner))

	list, _ := f.svc.ListMine(ctx, f.owner.ID)
	if list[0].ID != second.ID || list[1].ID != f.dm.ID {
		t.Fatalf("newest conversation is not first: %v then %v", list[0].ID, list[1].ID)
	}

	if _, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hello again"}); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	list, _ = f.svc.ListMine(ctx, f.owner.ID)
	if list[0].ID != f.dm.ID {
		t.Errorf("conversation with the newest message is not first: %v", list[0].ID)
	}
	if list[0].LastMessage == nil || list[0].LastMessage.Body.Text != "hello again" {
		t.Errorf("LastMessage = %+v, want the message just sent", list[0].LastMessage)
	}
	if list[1].LastMessage != nil {
		t.Errorf("silent conversation has a LastMessage: %+v", list[1].LastMessage)
	}
}

func TestGetMineDistinguishesMissingFromForbidden(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	got, err := f.svc.GetMine(ctx, f.owner.ID, f.dm.ID)
	if err != nil {
		t.Fatalf("member refused: %v", err)
	}
	if got.ID != f.dm.ID || len(got.Participants) != 2 {
		t.Errorf("GetMine = %+v, want the hydrated DM", got)
	}

	_, err = f.svc.GetMine(ctx, f.other.ID, f.dm.ID)
	if domain.KindOf(err) != domain.KindForbidden || domain.CodeOf(err) != "not_participant" {
		t.Errorf("non-member got %v, want forbidden/not_participant", err)
	}
	_, err = f.svc.GetMine(ctx, f.owner.ID, domain.NewID())
	if domain.CodeOf(err) != "conversation_not_found" {
		t.Errorf("missing conversation got %v, want conversation_not_found", err)
	}
}

// Names are read live, not copied into the conversation: a renamed agent is
// renamed everywhere, and a retired one is still named in the history it
// left behind.
func TestParticipantsReflectCurrentNamesAndSurviveDeletion(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	agentSvc := agents.New(f.db)

	newName := "Renamed Helper"
	if _, err := agentSvc.Update(ctx, f.owner.ID, f.agent.ID, agents.UpdateInput{DisplayName: &newName}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := agentSvc.Delete(ctx, f.owner.ID, f.agent.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := f.svc.GetMine(ctx, f.owner.ID, f.dm.ID)
	if err != nil {
		t.Fatalf("GetMine after agent deletion: %v", err)
	}
	var agent *conversations.Participant
	for i := range got.Participants {
		if got.Participants[i].Kind == conversations.ParticipantAgent {
			agent = &got.Participants[i]
		}
	}
	if agent == nil || agent.DisplayName != newName {
		t.Errorf("agent participant = %+v, want it present with the new name", agent)
	}
}
