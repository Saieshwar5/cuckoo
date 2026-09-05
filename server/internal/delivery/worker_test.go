package delivery_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// receiver is a backend under test: it records every request and answers
// with whatever status it has been told to.
type receiver struct {
	*httptest.Server
	status atomic.Int32
	mu     sync.Mutex
	got    []received
}

type received struct {
	header http.Header
	body   []byte
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	rc := &receiver{}
	rc.status.Store(http.StatusOK)
	rc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.got = append(rc.got, received{header: r.Header.Clone(), body: body})
		rc.mu.Unlock()
		w.WriteHeader(int(rc.status.Load()))
	}))
	t.Cleanup(rc.Close)
	return rc
}

func (rc *receiver) requests() []received {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]received(nil), rc.got...)
}

type fixture struct {
	db      *store.Store
	tx      pgx.Tx
	convs   *conversations.Service
	agents  *agents.Service
	svc     *delivery.Service
	worker  *delivery.Worker
	owner   users.User
	agent   agents.Agent
	binding agents.Binding
	secret  string
	dm      conversations.Conversation
	backend *receiver
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db, tx := testutil.NewStoreTx(t)
	f := &fixture{db: db, tx: tx, convs: conversations.New(db), agents: agents.New(db)}
	f.svc = delivery.New(db, f.convs)
	f.worker = delivery.NewWorker(db, f.svc, f.agents, delivery.Options{
		AllowLoopback: true,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	f.owner = testutil.CreateUser(t, db, testutil.WithDisplayName("Priya"))
	f.agent = testutil.CreateAgent(t, db, f.owner, testutil.WithHandle("sbi-support"), testutil.WithAgentName("SBI Support"))
	f.dm = testutil.OwnerDM(t, db, f.agent)
	f.backend = newReceiver(t)
	f.binding, f.secret = testutil.BindWebhook(t, db, f.agent, f.backend.URL)
	return f
}

func (f *fixture) send(t *testing.T, conv conversations.Conversation, text string) conversations.Message {
	t.Helper()
	msg, err := f.convs.SendText(context.Background(), f.owner.ID, conv.ID, text)
	if err != nil {
		t.Fatalf("SendText %q: %v", text, err)
	}
	return msg
}

func (f *fixture) runOnce(t *testing.T) int {
	t.Helper()
	n, err := f.worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	return n
}

func (f *fixture) deliveryOf(t *testing.T, msg conversations.Message) gen.MessageDelivery {
	t.Helper()
	rows, err := f.db.ListDeliveriesByMessage(context.Background(), msg.ID)
	if err != nil {
		t.Fatalf("ListDeliveriesByMessage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("message has %d deliveries, want 1: %+v", len(rows), rows)
	}
	return rows[0]
}

func (f *fixture) makeDue(t *testing.T, d gen.MessageDelivery) {
	t.Helper()
	_, err := f.tx.Exec(context.Background(),
		"UPDATE message_deliveries SET next_attempt_at = now() - interval '1 second' WHERE id = $1", d.ID)
	if err != nil {
		t.Fatalf("backdate next_attempt_at: %v", err)
	}
}

func (f *fixture) failureStreak(t *testing.T) *time.Time {
	t.Helper()
	var streak *time.Time
	err := f.tx.QueryRow(context.Background(),
		"SELECT failure_streak_started_at FROM agent_bindings WHERE id = $1", f.binding.ID).Scan(&streak)
	if err != nil {
		t.Fatalf("read failure streak: %v", err)
	}
	return streak
}

func (f *fixture) bindingStatus(t *testing.T) agents.Status {
	t.Helper()
	b, err := f.agents.ActiveBinding(context.Background(), f.owner.ID, f.agent.ID)
	if err != nil || b == nil {
		t.Fatalf("ActiveBinding: %v, %v", b, err)
	}
	return b.Status
}

func (f *fixture) messageStatus(t *testing.T, conv conversations.Conversation) conversations.DeliveryStatus {
	t.Helper()
	page, err := f.convs.ListMessages(context.Background(), f.owner.ID, conv.ID, conversations.ListMessagesInput{Limit: 1})
	if err != nil || len(page.Messages) == 0 {
		t.Fatalf("ListMessages: %v", err)
	}
	return page.Messages[0].DeliveryStatus
}

// eventJSON is the envelope as a backend decodes it.
type eventJSON struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	Data    struct {
		Conversation struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"conversation"`
		Message struct {
			ID     string `json:"id"`
			Sender struct {
				Kind        string `json:"kind"`
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"sender"`
			Body struct {
				Text string `json:"text"`
			} `json:"body"`
		} `json:"message"`
		Participants []struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
			IsMe bool   `json:"is_me"`
		} `json:"participants"`
	} `json:"data"`
}

// The whole contract in one pass: the request a backend receives, how it
// verifies it, and what the hub records once it answered.
func TestDeliversSignedEvent(t *testing.T) {
	f := setup(t)
	msg := f.send(t, f.dm, "my UPI payment failed")

	if n := f.runOnce(t); n != 1 {
		t.Fatalf("attempted %d deliveries, want 1", n)
	}
	reqs := f.backend.requests()
	if len(reqs) != 1 {
		t.Fatalf("backend received %d requests, want 1", len(reqs))
	}
	r := reqs[0]

	if got := r.header.Get(delivery.HeaderEvent); got != "message.created" {
		t.Errorf("%s = %q", delivery.HeaderEvent, got)
	}
	eventID := r.header.Get(delivery.HeaderEventID)
	if !strings.HasPrefix(eventID, "evt_") {
		t.Errorf("%s = %q, want evt_ prefix", delivery.HeaderEventID, eventID)
	}
	ts := r.header.Get(delivery.HeaderTimestamp)
	if _, err := strconv.ParseInt(ts, 10, 64); err != nil {
		t.Errorf("%s = %q, want unix seconds", delivery.HeaderTimestamp, ts)
	}
	// A backend derives the key with one hash of the secret it was given.
	if want := delivery.Sign(domain.HashSecret(f.secret), ts, r.body); r.header.Get(delivery.HeaderSignature) != want {
		t.Errorf("signature does not verify with sha256(secret) as key")
	}

	var env eventJSON
	if err := json.Unmarshal(r.body, &env); err != nil {
		t.Fatalf("decode body: %v\n%s", err, r.body)
	}
	if env.ID != eventID || env.Type != "message.created" {
		t.Errorf("envelope = %+v, want id %s and type message.created", env, eventID)
	}
	if env.AgentID != domain.FormatID(domain.PrefixAgent, f.agent.ID) {
		t.Errorf("agent_id = %q", env.AgentID)
	}
	if env.Data.Conversation.ID != domain.FormatID(domain.PrefixConv, f.dm.ID) || env.Data.Conversation.Kind != "dm" {
		t.Errorf("conversation = %+v", env.Data.Conversation)
	}
	m := env.Data.Message
	if m.ID != domain.FormatID(domain.PrefixMessage, msg.ID) || m.Body.Text != "my UPI payment failed" {
		t.Errorf("message = %+v", m)
	}
	if m.Sender.Kind != "user" || m.Sender.ID != domain.FormatID(domain.PrefixUser, f.owner.ID) || m.Sender.DisplayName != "Priya" {
		t.Errorf("sender = %+v", m.Sender)
	}
	me := 0
	for _, p := range env.Data.Participants {
		if p.IsMe {
			me++
			if p.Kind != "agent" || p.ID != env.AgentID {
				t.Errorf("is_me on the wrong participant: %+v", p)
			}
		}
	}
	if len(env.Data.Participants) != 2 || me != 1 {
		t.Errorf("participants = %+v, want both with is_me on the agent", env.Data.Participants)
	}

	d := f.deliveryOf(t, msg)
	if d.Status != "delivered" || d.Attempts != 1 || d.DeliveredAt == nil || d.LastError != nil {
		t.Errorf("delivery = %+v, want delivered on the first attempt", d)
	}
	if f.bindingStatus(t) != agents.StatusConnected {
		t.Errorf("binding status = %q, want connected after a delivery", f.bindingStatus(t))
	}
	if f.messageStatus(t, f.dm) != conversations.DeliveryDelivered {
		t.Errorf("message delivery status = %q, want delivered", f.messageStatus(t, f.dm))
	}

	if n := f.runOnce(t); n != 0 || len(f.backend.requests()) != 1 {
		t.Errorf("a delivered event was attempted again")
	}
}

func TestRetriesUntilTheBackendAnswers(t *testing.T) {
	f := setup(t)
	f.backend.status.Store(http.StatusInternalServerError)
	msg := f.send(t, f.dm, "hello?")

	f.runOnce(t)
	d := f.deliveryOf(t, msg)
	if d.Status != "pending" || d.Attempts != 1 || d.LastError == nil || !strings.Contains(*d.LastError, "500") {
		t.Fatalf("after a 500: delivery = %+v, want pending with the error recorded", d)
	}
	if !d.NextAttemptAt.After(d.CreatedAt) {
		t.Errorf("next attempt %v is not after creation %v", d.NextAttemptAt, d.CreatedAt)
	}
	if f.failureStreak(t) == nil {
		t.Error("failure streak not started")
	}
	if f.messageStatus(t, f.dm) != conversations.DeliveryPending {
		t.Errorf("message shows %q while retrying, want pending", f.messageStatus(t, f.dm))
	}

	if n := f.runOnce(t); n != 0 {
		t.Errorf("retried %d deliveries before they were due", n)
	}

	f.backend.status.Store(http.StatusOK)
	f.makeDue(t, d)
	if n := f.runOnce(t); n != 1 {
		t.Fatalf("attempted %d when one was due", n)
	}
	d = f.deliveryOf(t, msg)
	if d.Status != "delivered" || d.Attempts != 2 || d.LastError != nil {
		t.Errorf("after recovery: delivery = %+v, want delivered on attempt 2 with the error cleared", d)
	}
	if f.failureStreak(t) != nil {
		t.Error("failure streak survived a success")
	}
	if f.bindingStatus(t) != agents.StatusConnected {
		t.Errorf("binding status = %q after recovery", f.bindingStatus(t))
	}
}

func TestBindingBecomesUnreachableAfterFiveMinutesOfFailure(t *testing.T) {
	f := setup(t)
	f.backend.status.Store(http.StatusBadGateway)
	msg := f.send(t, f.dm, "anyone there?")
	f.runOnce(t)
	if f.bindingStatus(t) == agents.StatusUnreachable {
		t.Fatal("one failure made the binding unreachable")
	}

	if _, err := f.tx.Exec(context.Background(),
		"UPDATE agent_bindings SET failure_streak_started_at = now() - interval '6 minutes' WHERE id = $1",
		f.binding.ID); err != nil {
		t.Fatalf("backdate streak: %v", err)
	}
	f.makeDue(t, f.deliveryOf(t, msg))
	f.runOnce(t)
	if f.bindingStatus(t) != agents.StatusUnreachable {
		t.Errorf("binding status = %q after six minutes of failure, want unreachable", f.bindingStatus(t))
	}

	f.backend.status.Store(http.StatusOK)
	f.makeDue(t, f.deliveryOf(t, msg))
	f.runOnce(t)
	if f.bindingStatus(t) != agents.StatusConnected {
		t.Errorf("binding status = %q after recovery, want connected", f.bindingStatus(t))
	}
}

// Nobody to deliver to means the user is told straight away, not after a day
// of retries against nothing.
func TestFailsImmediatelyWithoutBinding(t *testing.T) {
	f := setup(t)
	unbound := testutil.CreateAgent(t, f.db, f.owner)
	dm := testutil.OwnerDM(t, f.db, unbound)

	msg := f.send(t, dm, "hello?")
	if msg.DeliveryStatus != conversations.DeliveryFailed {
		t.Errorf("sent message reports %q, want failed", msg.DeliveryStatus)
	}
	d := f.deliveryOf(t, msg)
	if d.Status != "failed" || d.LastError == nil || *d.LastError != "no_binding" {
		t.Errorf("delivery = %+v, want failed/no_binding", d)
	}
	if n := f.runOnce(t); n != 0 || len(f.backend.requests()) != 0 {
		t.Error("a failed delivery was attempted")
	}
}

func TestFailsWhenBindingIsRevokedAfterSend(t *testing.T) {
	f := setup(t)
	msg := f.send(t, f.dm, "hello?")
	if err := f.agents.RevokeBinding(context.Background(), f.owner.ID, f.agent.ID); err != nil {
		t.Fatalf("RevokeBinding: %v", err)
	}

	if n := f.runOnce(t); n != 0 {
		t.Errorf("attempted %d deliveries to a revoked binding", n)
	}
	d := f.deliveryOf(t, msg)
	if d.Status != "failed" || d.LastError == nil || *d.LastError != "no_binding" {
		t.Errorf("delivery = %+v, want failed/no_binding", d)
	}
	if f.messageStatus(t, f.dm) != conversations.DeliveryFailed {
		t.Errorf("message shows %q, want failed", f.messageStatus(t, f.dm))
	}
}

func TestExpiresAfterADay(t *testing.T) {
	f := setup(t)
	f.backend.status.Store(http.StatusInternalServerError)
	msg := f.send(t, f.dm, "hello?")
	if _, err := f.tx.Exec(context.Background(),
		"UPDATE message_deliveries SET created_at = now() - interval '25 hours' WHERE id = $1",
		f.deliveryOf(t, msg).ID); err != nil {
		t.Fatalf("backdate created_at: %v", err)
	}

	if n := f.runOnce(t); n != 0 || len(f.backend.requests()) != 0 {
		t.Error("an expired delivery was attempted")
	}
	d := f.deliveryOf(t, msg)
	if d.Status != "failed" || d.LastError == nil || *d.LastError != "expired" {
		t.Errorf("delivery = %+v, want failed/expired", d)
	}
}

// Socket-bound agents are the socket transport's job. Their deliveries wait,
// untouched, and burn no attempts.
func TestLeavesSocketBoundDeliveriesAlone(t *testing.T) {
	f := setup(t)
	socketAgent := testutil.CreateAgent(t, f.db, f.owner)
	testutil.BindAgent(t, f.db, socketAgent)
	dm := testutil.OwnerDM(t, f.db, socketAgent)

	msg := f.send(t, dm, "over the socket")
	if n := f.runOnce(t); n != 0 {
		t.Errorf("attempted %d socket deliveries over webhooks", n)
	}
	d := f.deliveryOf(t, msg)
	if d.Status != "pending" || d.Attempts != 0 {
		t.Errorf("delivery = %+v, want pending and unattempted", d)
	}
}

func TestDeliversABatchInOneRun(t *testing.T) {
	f := setup(t)
	for _, text := range []string{"one", "two", "three"} {
		f.send(t, f.dm, text)
	}

	if n := f.runOnce(t); n != 3 {
		t.Fatalf("attempted %d, want 3", n)
	}
	reqs := f.backend.requests()
	seen := map[string]bool{}
	for _, r := range reqs {
		seen[r.header.Get(delivery.HeaderEventID)] = true
	}
	if len(reqs) != 3 || len(seen) != 3 {
		t.Errorf("backend received %d requests with %d distinct event ids, want 3 and 3", len(reqs), len(seen))
	}
}
