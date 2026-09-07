package retention_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/blobs"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/retention"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

const day = 24 * time.Hour

type fixture struct {
	db    *store.Store
	blobs blobs.Store
	conv  *conversations.Service
	owner users.User
	dm    conversations.Conversation
	agent uuid.UUID
	now   time.Time
}

// setup builds a person, their agent, and their DM, with a clock the test
// moves rather than waits on.
func setup(t *testing.T) *fixture {
	t.Helper()
	db := testutil.NewStore(t)
	bs, err := blobs.NewDisk(t.TempDir())
	if err != nil {
		t.Fatalf("blobs: %v", err)
	}
	owner := testutil.CreateUser(t, db)
	agent := testutil.CreateAgent(t, db, owner)
	return &fixture{
		db: db, blobs: bs, conv: conversations.New(db), owner: owner,
		dm: testutil.OwnerDM(t, db, agent), agent: agent.ID, now: time.Now(),
	}
}

func (f *fixture) service(t *testing.T, p retention.Policy, ahead time.Duration) *retention.Service {
	t.Helper()
	return retention.New(f.db, f.blobs, p, retention.WithClock(func() time.Time { return f.now.Add(ahead) }))
}

func (f *fixture) say(t *testing.T, text string) conversations.Message {
	t.Helper()
	res, err := f.conv.SendAsUser(context.Background(), f.owner.ID, f.dm.ID, conversations.SendInput{Text: text})
	if err != nil {
		t.Fatalf("send %q: %v", text, err)
	}
	return res.Message
}

// attach makes a file of the given size the owner sent with msg, bytes and all.
func (f *fixture) attach(t *testing.T, msg conversations.Message, size int64) gen.Medium {
	t.Helper()
	ctx := context.Background()
	id := domain.NewID()
	key := "test/" + id.String()
	if err := f.blobs.Put(ctx, key, bytes.NewReader(bytes.Repeat([]byte("x"), int(size)))); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	m, err := f.db.CreateMedia(ctx, gen.CreateMediaParams{
		ID: id, OwnerKind: retention.OwnerUser, OwnerID: f.owner.ID, Kind: "file",
		MimeType: "application/octet-stream", ByteSize: size, FileName: "f.bin", StorageKey: key,
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}
	n, err := f.db.ClaimMedia(ctx, gen.ClaimMediaParams{
		MessageID: &msg.ID, Ids: []uuid.UUID{id}, OwnerKind: retention.OwnerUser, OwnerID: f.owner.ID,
	})
	if err != nil || n != 1 {
		t.Fatalf("claim media: n=%d err=%v", n, err)
	}
	return m
}

func (f *fixture) messageExists(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	_, err := f.db.GetMessage(context.Background(), id)
	if err != nil && !store.IsNoRows(err) {
		t.Fatalf("get message: %v", err)
	}
	return err == nil
}

func (f *fixture) mediaExists(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	_, err := f.db.GetMedia(context.Background(), id)
	if err != nil && !store.IsNoRows(err) {
		t.Fatalf("get media: %v", err)
	}
	return err == nil
}

func (f *fixture) blobExists(t *testing.T, key string) bool {
	t.Helper()
	rc, err := f.blobs.Open(context.Background(), key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "not") {
			return false
		}
		t.Fatalf("open blob: %v", err)
	}
	_ = rc.Close()
	return true
}

var window = retention.Policy{MessageAge: 90 * day, UserMediaBudget: 100 << 20, AgentMediaBudget: 1 << 30}

func TestExpiredMessagesGoWithTheirFilesAndDeliveries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m1 := f.say(t, "first")
	m2 := f.say(t, "second")
	file := f.attach(t, m1, 10)
	if rows, err := f.db.ListDeliveriesByMessage(ctx, m1.ID); err != nil || len(rows) == 0 {
		t.Fatalf("no delivery rows for m1: %v", err)
	}

	// Eighty-nine days on, everything is inside the window.
	r, err := f.service(t, window, 89*day).Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if r.Messages != 0 || r.Files != 0 || !f.messageExists(t, m1.ID) {
		t.Fatalf("a sweep inside the window removed something: %+v", r)
	}

	// Ninety-one days on, it is all past it.
	r, err = f.service(t, window, 91*day).Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if r.Messages != 2 || r.Files != 1 {
		t.Errorf("report = %+v, want 2 messages and 1 file", r)
	}
	if f.messageExists(t, m1.ID) || f.messageExists(t, m2.ID) {
		t.Error("expired messages are still there")
	}
	if f.mediaExists(t, file.ID) {
		t.Error("the expired message's file row is still there")
	}
	if f.blobExists(t, file.StorageKey) {
		t.Error("the expired message's bytes are still on disk")
	}
	if rows, err := f.db.ListDeliveriesByMessage(ctx, m1.ID); err != nil || len(rows) != 0 {
		t.Errorf("delivery rows outlived their message: %d, %v", len(rows), err)
	}
}

func TestZeroAgeKeepsEverything(t *testing.T) {
	f := setup(t)
	m := f.say(t, "kept")
	svc := f.service(t, retention.Policy{}, 1000*day)
	if _, ok := svc.Window(); ok {
		t.Fatal("a zero age has a window")
	}
	if _, err := svc.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if !f.messageExists(t, m.ID) {
		t.Error("a hub that keeps everything removed a message")
	}
}

func TestDryRunCountsAndRemovesNothing(t *testing.T) {
	f := setup(t)
	m := f.say(t, "still here")
	f.attach(t, m, 200)
	p := window
	p.DryRun = true
	p.UserMediaBudget = 100
	r, err := f.service(t, p, 91*day).Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if !r.DryRun || r.Messages != 1 || r.OverBudget != 1 {
		t.Errorf("dry run report = %+v, want 1 message and 1 owner over budget", r)
	}
	if !f.messageExists(t, m.ID) {
		t.Error("a dry run deleted a message")
	}
}

func TestOverBudgetLosesOldestFilesFirst(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m := f.say(t, "two files")
	old := f.attach(t, m, 60)
	// The second is newer by clock; the sort is by created_at then id, and
	// both are set by the database in order.
	recent := f.attach(t, m, 70)

	p := window
	p.UserMediaBudget = 100
	r, err := f.service(t, p, 0).Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if r.OverBudget != 1 || r.Files != 1 {
		t.Errorf("report = %+v, want 1 owner over budget and 1 file removed", r)
	}
	if f.mediaExists(t, old.ID) {
		t.Error("the oldest file survived")
	}
	if !f.mediaExists(t, recent.ID) {
		t.Error("the newer file went too")
	}
	if !f.messageExists(t, m.ID) {
		t.Error("the message lost its words along with a file")
	}
	usage, err := f.service(t, p, 0).Usage(ctx, retention.OwnerUser, f.owner.ID)
	if err != nil || usage.UsedBytes != 70 || usage.BudgetBytes != 100 {
		t.Errorf("usage = %+v, %v; want 70 of 100", usage, err)
	}
}

func TestProfilePicturesAreNeverCounted(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	face := domain.NewID()
	if _, err := f.db.CreateMedia(ctx, gen.CreateMediaParams{
		ID: face, OwnerKind: retention.OwnerUser, OwnerID: f.owner.ID, Kind: "image",
		MimeType: "image/png", ByteSize: 5000, FileName: "me.png", StorageKey: "faces/" + face.String(),
	}); err != nil {
		t.Fatalf("create face: %v", err)
	}
	p := window
	p.UserMediaBudget = 100
	r, err := f.service(t, p, 500*day).Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if r.OverBudget != 0 || r.Files != 0 || !f.mediaExists(t, face) {
		t.Errorf("a profile picture was counted or removed: %+v", r)
	}
	usage, _ := f.service(t, p, 0).Usage(ctx, retention.OwnerUser, f.owner.ID)
	if usage.UsedBytes != 0 {
		t.Errorf("a face counts against the budget: %d", usage.UsedBytes)
	}
}

func TestFinishedDeliveriesArePrunedAfterAWeek(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m := f.say(t, "delivered")
	rows, err := f.db.ListDeliveriesByMessage(ctx, m.ID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("no deliveries: %v", err)
	}
	if err := f.db.MarkDelivered(ctx, rows[0].ID); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if r, _ := f.service(t, window, 6*day).Sweep(ctx); r.Deliveries != 0 {
		t.Errorf("a six-day-old record was pruned: %+v", r)
	}
	r, err := f.service(t, window, 8*day).Sweep(ctx)
	if err != nil || r.Deliveries != 1 {
		t.Errorf("an eight-day-old record was not pruned: %+v, %v", r, err)
	}
	if !f.messageExists(t, m.ID) {
		t.Error("pruning a delivery record took the message with it")
	}
}

func TestTrimmed(t *testing.T) {
	f := setup(t)
	svc := f.service(t, window, 0)
	if svc.Trimmed(f.now.Add(-1 * day)) {
		t.Error("a conversation from yesterday reads as trimmed")
	}
	if !svc.Trimmed(f.now.Add(-100 * day)) {
		t.Error("a conversation from a hundred days ago does not read as trimmed")
	}
	if f.service(t, retention.Policy{}, 0).Trimmed(f.now.Add(-1000 * day)) {
		t.Error("a hub that keeps everything reports trimming")
	}
}
