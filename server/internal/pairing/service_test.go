package pairing_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

type fixture struct {
	db     *store.Store
	svc    *pairing.Service
	convs  *conversations.Service
	events *delivery.Service
	owner  users.User
	priya  users.User
	agent  agents.Agent
	secret string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db := testutil.NewStore(t)
	agentService := agents.New(db)
	convs := conversations.New(db)
	f := &fixture{
		db:     db,
		convs:  convs,
		events: delivery.New(db, convs),
		svc:    pairing.New(db, agentService, convs, users.New(db), "https://hub.test/"),
		owner:  testutil.CreateUser(t, db, testutil.WithDisplayName("SBI")),
		priya:  testutil.CreateUser(t, db, testutil.WithDisplayName("Priya")),
	}
	f.agent = testutil.CreateAgent(t, db, f.owner, testutil.WithHandle("sbi-support"), testutil.WithAgentName("SBI Support"))
	_, f.secret = testutil.BindAgent(t, db, f.agent)
	return f
}

func code(t *testing.T, err error) string {
	t.Helper()
	derr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("not a classified error: %v", err)
	}
	return derr.Code
}

// The whole story: the owner mints a personalised code, a stranger resolves
// it, adds the agent, gets a chat, and the backend hears who arrived.
func TestScanAddsAgentAndTellsBackend(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	one := 1
	tok, plain, err := f.svc.CreateToken(ctx, f.owner.ID, f.agent.ID, pairing.CreateTokenInput{
		Payload: json.RawMessage(`{"customer_ref":"SBI-8812"}`), MaxUses: &one,
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if !strings.HasPrefix(plain, "pair_") || f.svc.URL(plain) != "https://hub.test/p/"+plain {
		t.Errorf("plaintext %q, url %q", plain, f.svc.URL(plain))
	}

	card, err := f.svc.Resolve(ctx, f.priya.ID, plain)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card.Agent.ID != f.agent.ID || card.OwnerName != "SBI" || card.AlreadyAdded || card.Status == nil {
		t.Errorf("card = %+v", card)
	}

	accepted, err := f.svc.Accept(ctx, f.priya.ID, plain)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if !accepted.New || accepted.Conversation.Kind != conversations.KindDM {
		t.Errorf("accepted = %+v", accepted)
	}
	// Priya now sees the chat; the owner still sees only their own.
	mine, _ := f.convs.ListMine(ctx, f.priya.ID)
	if len(mine) != 1 || mine[0].ID != accepted.Conversation.ID {
		t.Errorf("priya's chats = %+v", mine)
	}
	contacts, _ := f.svc.Contacts(ctx, f.priya.ID)
	if len(contacts) != 1 || contacts[0].AgentID != f.agent.ID || contacts[0].AddedVia != "pair_token" || contacts[0].OwnerName != "SBI" {
		t.Errorf("contacts = %+v", contacts)
	}

	// The backend hears it, with the payload.
	evs, err := f.events.ListEvents(ctx, f.agent.ID, delivery.ListEventsInput{})
	if err != nil || len(evs) != 1 || evs[0].Type != events.TypeConversationJoined {
		t.Fatalf("events = %+v, %v", evs, err)
	}
	joined := evs[0].Data.(events.ConversationJoined)
	var payload map[string]string
	if joined.PairToken != nil {
		_ = json.Unmarshal(joined.PairToken.Payload, &payload)
	}
	if joined.PairToken == nil || joined.PairToken.ID != domain.FormatID(domain.PrefixToken, tok.ID) ||
		payload["customer_ref"] != "SBI-8812" {
		t.Errorf("joined = %+v", joined)
	}
	names := map[string]bool{}
	for _, p := range joined.Participants {
		names[p.DisplayName] = p.Kind == "agent" && p.IsMe || p.Kind == "user"
	}
	if len(joined.Participants) != 2 || !names["Priya"] || !names["SBI Support"] {
		t.Errorf("participants = %+v", joined.Participants)
	}

	// Same person again: same chat, no use spent, not new.
	again, err := f.svc.Accept(ctx, f.priya.ID, plain)
	if err != nil || again.New || again.Conversation.ID != accepted.Conversation.ID {
		t.Errorf("second accept = %+v, %v", again, err)
	}
	// Someone else: the single use is gone.
	ravi := testutil.CreateUser(t, f.db, testutil.WithDisplayName("Ravi"))
	if _, err := f.svc.Accept(ctx, ravi.ID, plain); code(t, err) != "token_spent" {
		t.Errorf("third person got %v, want token_spent", err)
	}
	list, _ := f.svc.ListTokens(ctx, f.owner.ID, f.agent.ID)
	if len(list) != 1 || list[0].UseCount != 1 {
		t.Errorf("tokens = %+v", list)
	}
}

func TestTokensStopWorking(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	_, revoked, _ := f.svc.CreateToken(ctx, f.owner.ID, f.agent.ID, pairing.CreateTokenInput{})
	tok, _ := f.svc.ListTokens(ctx, f.owner.ID, f.agent.ID)
	if err := f.svc.RevokeToken(ctx, f.owner.ID, f.agent.ID, tok[0].ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if _, err := f.svc.Resolve(ctx, f.priya.ID, revoked); code(t, err) != "token_revoked" {
		t.Errorf("revoked: %v", err)
	}

	past := -time.Second
	if _, _, err := f.svc.CreateToken(ctx, f.owner.ID, f.agent.ID, pairing.CreateTokenInput{ExpiresIn: &past}); code(t, err) != "invalid_expires_in" {
		t.Errorf("past expiry accepted: %v", err)
	}
	if _, err := f.svc.Resolve(ctx, f.priya.ID, "pair_nonsense"); code(t, err) != "invalid_token" {
		t.Errorf("garbage: %v", err)
	}
	if _, err := f.svc.Resolve(ctx, f.priya.ID, "https://hub.test/p/whatever"); code(t, err) != "invalid_token" {
		t.Errorf("url instead of code: %v", err)
	}
	// Not the owner: no minting, no listing.
	if _, _, err := f.svc.CreateToken(ctx, f.priya.ID, f.agent.ID, pairing.CreateTokenInput{}); code(t, err) != "not_owner" {
		t.Errorf("stranger minted: %v", err)
	}
}

// Blocking closes the chat both ways and tells the backend; adding the
// agent again reopens it.
func TestBlockClosesTheChat(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	_, plain, _ := f.svc.CreateToken(ctx, f.owner.ID, f.agent.ID, pairing.CreateTokenInput{})
	accepted, _ := f.svc.Accept(ctx, f.priya.ID, plain)
	dm := accepted.Conversation.ID

	if err := f.svc.Block(ctx, f.priya.ID, f.agent.ID); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if _, err := f.convs.SendAsAgent(ctx, f.agent.ID, dm, conversations.SendInput{Text: "hello?"}); code(t, err) != "blocked" {
		t.Errorf("agent send after block: %v", err)
	}
	if _, err := f.convs.SendAsUser(ctx, f.priya.ID, dm, conversations.SendInput{Text: "hello?"}); code(t, err) != "blocked" {
		t.Errorf("user send after block: %v", err)
	}
	evs, _ := f.events.ListEvents(ctx, f.agent.ID, delivery.ListEventsInput{})
	if len(evs) != 2 || evs[1].Type != events.TypeConversationLeft || evs[1].Data.(events.ConversationLeft).Reason != "user_blocked" {
		t.Errorf("events after block = %+v", evs)
	}
	card, _ := f.svc.Resolve(ctx, f.priya.ID, plain)
	if !card.AlreadyAdded || !card.Blocked {
		t.Errorf("card after block = %+v", card)
	}

	// Scanning again is the way back in.
	again, err := f.svc.Accept(ctx, f.priya.ID, plain)
	if err != nil || again.New {
		t.Fatalf("re-accept = %+v, %v", again, err)
	}
	if _, err := f.convs.SendAsUser(ctx, f.priya.ID, dm, conversations.SendInput{Text: "back"}); err != nil {
		t.Errorf("send after unblock: %v", err)
	}
	evs, _ = f.events.ListEvents(ctx, f.agent.ID, delivery.ListEventsInput{})
	if len(evs) != 4 || evs[2].Type != events.TypeConversationJoined || evs[2].Data.(events.ConversationJoined).PairToken != nil {
		t.Errorf("events after re-add = %d %+v", len(evs), evs)
	}
	// Blocking a stranger's agent you never added is not a thing.
	if err := f.svc.Block(ctx, f.priya.ID, testutil.CreateAgent(t, f.db, f.owner).ID); code(t, err) != "not_a_contact" {
		t.Errorf("block non-contact: %v", err)
	}
}
