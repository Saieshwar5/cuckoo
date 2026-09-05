package realtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// subscriptionBuffer is how far a connection may fall behind before it is
// dropped. A phone that cannot drain this many events is not keeping up, and
// it will catch up from the database on reconnect anyway; holding events for
// it would only stall the dispatch of everyone else's.
const subscriptionBuffer = 64

// Hub delivers events from a Bus to the connections on this instance.
type Hub struct {
	log *slog.Logger

	mu     sync.Mutex
	subs   map[uuid.UUID]map[*Subscription]struct{}
	closed bool
}

// NewHub builds an idle hub. Start it to receive from a bus.
func NewHub(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{log: log.With("component", "realtime"), subs: map[uuid.UUID]map[*Subscription]struct{}{}}
}

// Subscription is one connection's view of the events for one principal.
//
// C closes when the hub is closed, or when the subscription was dropped for
// not keeping up; either way the connection should end and the client will
// reconnect and catch up. The connection stays registered until it calls
// Close itself, which is how Wait knows the connection has finished.
type Subscription struct {
	C          <-chan Event
	c          chan Event
	userID     uuid.UUID
	hub        *Hub
	signalOnce sync.Once
	detachOnce sync.Once
}

// Close detaches the subscription. Safe to call more than once. A
// connection defers this last, after any bookkeeping it does on the way
// out, so that Wait returning means that bookkeeping is done too.
func (s *Subscription) Close() {
	s.detachOnce.Do(func() { s.hub.detach(s) })
	s.signal()
}

// signal ends C without detaching.
func (s *Subscription) signal() {
	s.signalOnce.Do(func() { close(s.c) })
}

// Subscribe attaches a connection for the given person.
func (h *Hub) Subscribe(userID uuid.UUID) *Subscription {
	c := make(chan Event, subscriptionBuffer)
	s := &Subscription{C: c, c: c, userID: userID, hub: h}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[userID] == nil {
		h.subs[userID] = map[*Subscription]struct{}{}
	}
	h.subs[userID][s] = struct{}{}
	return s
}

func (h *Hub) detach(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.subs[s.userID]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(h.subs, s.userID)
		}
	}
}

// Start subscribes to the bus and dispatches until ctx ends. It returns once
// the subscription is live.
func (h *Hub) Start(ctx context.Context, bus Bus) error {
	events, err := bus.Subscribe(ctx)
	if err != nil {
		return err
	}
	go func() {
		for ev := range events {
			h.Dispatch(ev)
		}
	}()
	return nil
}

// Dispatch hands an event to every connection of every principal it names.
// A connection that cannot take it is dropped rather than waited for.
func (h *Hub) Dispatch(ev Event) {
	var slow []*Subscription

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	for _, userID := range ev.Recipients {
		for s := range h.subs[userID] {
			select {
			case s.c <- ev:
			default:
				slow = append(slow, s)
			}
		}
	}
	h.mu.Unlock()

	for _, s := range slow {
		h.log.Warn("dropping connection that is not keeping up", "user", s.userID)
		s.Close()
	}
}

// Close tells every connection to end, for shutdown. Nothing is dispatched
// afterwards. Connections detach themselves as they finish; Wait for them.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	var all []*Subscription
	for _, set := range h.subs {
		for s := range set {
			all = append(all, s)
		}
	}
	h.mu.Unlock()

	for _, s := range all {
		s.signal()
	}
}

// Wait blocks until every connection has detached, or ctx ends.
//
// A server's HTTP shutdown does not wait for hijacked connections, so
// without this a socket handler could still be finishing its bookkeeping
// after the process, or a test, believes everything has stopped.
func (h *Hub) Wait(ctx context.Context) error {
	for {
		h.mu.Lock()
		n := len(h.subs)
		h.mu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
