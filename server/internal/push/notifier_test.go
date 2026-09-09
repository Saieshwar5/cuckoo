package push_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/push"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// A notification is the most intrusive thing this software does: it interrupts
// somebody who did not ask, on a device in their pocket, possibly at night.
// Every test here is a rule about when not to.

type recorder struct {
	sent []push.Message
	// gone marks a token Expo says no longer exists.
	gone map[string]bool
}

func (r *recorder) Send(_ context.Context, messages []push.Message) ([]push.Result, error) {
	r.sent = append(r.sent, messages...)
	out := make([]push.Result, 0, len(messages))
	for _, m := range messages {
		out = append(out, push.Result{Token: m.To, Gone: r.gone[m.To]})
	}
	return out, nil
}

// watching is a presence that says exactly who has the app open.
type watching map[uuid.UUID]bool

func (w watching) Arrived(context.Context, uuid.UUID) error { return nil }
func (w watching) Left(context.Context, uuid.UUID) error    { return nil }
func (w watching) Watching(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	for _, id := range ids {
		out[id] = w[id]
	}
	return out, nil
}

type fixture struct {
	db    *store.Store
	sends *recorder
	seen  watching
}

func setup(t *testing.T) *fixture {
	t.Helper()
	return &fixture{db: testutil.NewStore(t), sends: &recorder{gone: map[string]bool{}}, seen: watching{}}
}

func (f *fixture) notifier() *push.Notifier {
	return push.New(f.db, f.seen, f.sends, nil)
}

// device gives a person a phone that can be woken.
func (f *fixture) device(t *testing.T, token string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	user := testutil.CreateUser(t, f.db)
	session, err := f.db.CreateSession(ctx, gen.CreateSessionParams{
		ID: domain.NewID(), UserID: user.ID, TokenHash: make([]byte, 32),
		DeviceName: "Pixel", ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := f.db.RegisterSessionPush(ctx, gen.RegisterSessionPushParams{
		PushToken: &token, PushPlatform: "android", ID: session.ID,
	}); err != nil {
		t.Fatalf("register push: %v", err)
	}
	return user.ID, session.ID
}

// reachable is how many devices of this person still have an address.
func (f *fixture) reachable(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	rows, err := f.db.PushTargets(context.Background(), gen.PushTargetsParams{UserIds: []uuid.UUID{userID}})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return len(rows)
}

func TestAMessageWakesAPhoneThatIsNotLooking(t *testing.T) {
	f := setup(t)
	person, _ := f.device(t, "ExponentPushToken[away]")
	agent := testutil.CreateAgent(t, f.db, testutil.CreateUser(t, f.db), testutil.WithAgentName("Weather"))

	f.notifier().MessageLanded(context.Background(), conversations.Landed{
		ConversationID: uuid.New(), MessageID: uuid.New(), AgentID: agent.ID,
		Text: "It will rain tomorrow.", Recipients: []uuid.UUID{person}, SenderID: agent.ID,
	})

	if len(f.sends.sent) != 1 {
		t.Fatalf("sent %d notifications, want 1", len(f.sends.sent))
	}
	got := f.sends.sent[0]
	if got.To != "ExponentPushToken[away]" {
		t.Errorf("sent to %q", got.To)
	}
	// The title is who said it, and the body is what they said.
	if got.Title != "Weather" || got.Body != "It will rain tomorrow." {
		t.Errorf("notification = %q / %q", got.Title, got.Body)
	}
	// And a tap has somewhere to go.
	if got.Data["conversation_id"] == "" {
		t.Error("nothing to open on a tap")
	}
}

func TestSomebodyWatchingIsNotInterrupted(t *testing.T) {
	f := setup(t)
	person, _ := f.device(t, "ExponentPushToken[here]")
	f.seen[person] = true
	agent := testutil.CreateAgent(t, f.db, testutil.CreateUser(t, f.db))

	f.notifier().MessageLanded(context.Background(), conversations.Landed{
		ConversationID: uuid.New(), AgentID: agent.ID, Text: "hello",
		Recipients: []uuid.UUID{person}, SenderID: agent.ID,
	})

	// They can already see it. Buzzing as well is how people learn to turn
	// notifications off and never turn them back on.
	if len(f.sends.sent) != 0 {
		t.Fatalf("interrupted somebody who was already reading it")
	}
}

func TestNobodyIsToldAboutTheirOwnMessage(t *testing.T) {
	f := setup(t)
	person, _ := f.device(t, "ExponentPushToken[self]")

	f.notifier().MessageLanded(context.Background(), conversations.Landed{
		ConversationID: uuid.New(), AgentID: uuid.New(), Text: "what I said",
		Recipients: []uuid.UUID{person}, SenderID: person,
	})

	if len(f.sends.sent) != 0 {
		t.Fatal("notified somebody about their own message")
	}
}

func TestAMessageWithNoWordsButAFileStillSaysSomething(t *testing.T) {
	f := setup(t)
	person, _ := f.device(t, "ExponentPushToken[file]")
	agent := testutil.CreateAgent(t, f.db, testutil.CreateUser(t, f.db))

	f.notifier().MessageLanded(context.Background(), conversations.Landed{
		ConversationID: uuid.New(), AgentID: agent.ID, Text: "",
		Recipients: []uuid.UUID{person}, SenderID: agent.ID, HasAttachments: true,
	})

	if len(f.sends.sent) != 1 || f.sends.sent[0].Body == "" {
		t.Fatal("a file with no caption should still be worth a line")
	}
}

func TestAMessageWithNothingInItIsNotWorthWakingAnybody(t *testing.T) {
	f := setup(t)
	person, _ := f.device(t, "ExponentPushToken[empty]")

	f.notifier().MessageLanded(context.Background(), conversations.Landed{
		ConversationID: uuid.New(), AgentID: uuid.New(), Text: "   ",
		Recipients: []uuid.UUID{person}, SenderID: uuid.New(),
	})

	if len(f.sends.sent) != 0 {
		t.Fatal("woke a phone for nothing")
	}
}

func TestADeadTokenIsForgotten(t *testing.T) {
	f := setup(t)
	person, session := f.device(t, "ExponentPushToken[gone]")
	f.sends.gone["ExponentPushToken[gone]"] = true
	agent := testutil.CreateAgent(t, f.db, testutil.CreateUser(t, f.db))

	f.notifier().MessageLanded(context.Background(), conversations.Landed{
		ConversationID: uuid.New(), AgentID: agent.ID, Text: "hello",
		Recipients: []uuid.UUID{person}, SenderID: agent.ID,
	})

	// Expo said the app is gone from that device. Keeping the address means
	// sending there for as long as the account exists.
	if n := f.reachable(t, person); n != 0 {
		t.Errorf("a dead token was kept: %d still reachable (session %s)", n, session)
	}
}

// The rule that matters most. Mute is what stands between an agent that is
// too talkative and an agent that is blocked for good, and a mute that still
// buzzes is not a mute.
func TestAMutedAgentDoesNotBuzz(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	person, _ := f.device(t, "ExponentPushToken[muted]")
	agent := testutil.CreateAgent(t, f.db, testutil.CreateUser(t, f.db))

	conv, err := conversations.New(f.db).CreateDM(ctx, person, agent.ID)
	if err != nil {
		t.Fatalf("dm: %v", err)
	}
	if _, err := f.db.CreateContact(ctx, gen.CreateContactParams{
		UserID: person, AgentID: agent.ID, DmConversationID: conv.ID, AddedVia: "catalogue",
	}); err != nil {
		t.Fatalf("contact: %v", err)
	}
	until := time.Now().Add(8 * time.Hour)
	if _, err := f.db.SetContactMuted(ctx, gen.SetContactMutedParams{
		UserID: person, AgentID: agent.ID, MutedUntil: &until,
	}); err != nil {
		t.Fatalf("mute: %v", err)
	}

	f.notifier().MessageLanded(ctx, conversations.Landed{
		ConversationID: conv.ID, AgentID: agent.ID, Text: "still here",
		Recipients: []uuid.UUID{person}, SenderID: agent.ID,
	})

	if len(f.sends.sent) != 0 {
		t.Fatal("a muted agent buzzed a phone")
	}
}

func TestPresenceThatCannotBeReadNotifiesAnyway(t *testing.T) {
	f := setup(t)
	person, _ := f.device(t, "ExponentPushToken[unknown]")
	agent := testutil.CreateAgent(t, f.db, testutil.CreateUser(t, f.db))
	n := push.New(f.db, brokenPresence{}, f.sends, nil)

	n.MessageLanded(context.Background(), conversations.Landed{
		ConversationID: uuid.New(), AgentID: agent.ID, Text: "hello",
		Recipients: []uuid.UUID{person}, SenderID: agent.ID,
	})

	// An unnecessary buzz is a worse app; a missing one is a broken promise.
	if len(f.sends.sent) != 1 {
		t.Fatal("stayed silent because presence was unavailable")
	}
}

type brokenPresence struct{}

func (brokenPresence) Arrived(context.Context, uuid.UUID) error { return nil }
func (brokenPresence) Left(context.Context, uuid.UUID) error    { return nil }
func (brokenPresence) Watching(context.Context, []uuid.UUID) (map[uuid.UUID]bool, error) {
	return nil, context.DeadlineExceeded
}
