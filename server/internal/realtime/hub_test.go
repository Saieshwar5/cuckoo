package realtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func event(t *testing.T, userIDs ...uuid.UUID) realtime.Event {
	t.Helper()
	ev, err := realtime.NewEvent("test.event", userIDs, map[string]string{"hello": "world"})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestHubDispatchesToNamedUsersOnly(t *testing.T) {
	hub := realtime.NewHub(nil)
	alice, bob := domain.NewID(), domain.NewID()
	a1, a2, b := hub.Subscribe(alice), hub.Subscribe(alice), hub.Subscribe(bob)
	defer a1.Close()
	defer a2.Close()
	defer b.Close()

	hub.Dispatch(event(t, alice))

	for name, s := range map[string]*realtime.Subscription{"alice's first": a1, "alice's second": a2} {
		select {
		case ev := <-s.C:
			var payload map[string]string
			if err := json.Unmarshal(ev.Payload, &payload); err != nil || payload["hello"] != "world" {
				t.Errorf("%s got payload %s", name, ev.Payload)
			}
		case <-time.After(time.Second):
			t.Errorf("%s connection heard nothing", name)
		}
	}
	select {
	case ev := <-b.C:
		t.Errorf("bob heard alice's event: %+v", ev)
	default:
	}
}

// A connection that stops draining is dropped, not waited for: its channel
// closes, which the socket handler turns into a close frame.
func TestHubDropsSlowSubscriber(t *testing.T) {
	hub := realtime.NewHub(nil)
	alice := domain.NewID()
	slow := hub.Subscribe(alice)
	ev := event(t, alice)

	for range 100 {
		hub.Dispatch(ev)
	}
	closed := false
	for range 200 {
		if _, open := <-slow.C; !open {
			closed = true
			break
		}
	}
	if !closed {
		t.Error("slow subscription was never dropped")
	}

	fresh := hub.Subscribe(alice)
	defer fresh.Close()
	hub.Dispatch(ev)
	select {
	case <-fresh.C:
	case <-time.After(time.Second):
		t.Error("new subscription after a drop heard nothing")
	}
}

func TestHubCloseEndsSubscriptions(t *testing.T) {
	hub := realtime.NewHub(nil)
	s := hub.Subscribe(domain.NewID())
	hub.Close()
	if _, open := <-s.C; open {
		t.Error("subscription still open after hub.Close")
	}
	s.Close() // idempotent
}

// The whole path over a real Redis: subscribe, publish, receive.
func TestRedisBusRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := testutil.NewBus(t)

	events, err := bus.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	alice := domain.NewID()
	if err := bus.Publish(ctx, event(t, alice)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Type != "test.event" || len(ev.UserIDs) != 1 || ev.UserIDs[0] != alice {
			t.Errorf("received %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("published event never arrived")
	}

	// Another test's bus is another channel.
	other := testutil.NewBus(t)
	if err := other.Publish(ctx, event(t)); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-events:
		t.Errorf("heard another bus's event: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}
