package realtime

import (
	"context"
	"log/slog"
	"sync"

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

	mu   sync.Mutex
	subs map[uuid.UUID]map[*Subscription]struct{}
}

// NewHub builds an idle hub. Start it to receive from a bus.
func NewHub(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{log: log.With("component", "realtime"), subs: map[uuid.UUID]map[*Subscription]struct{}{}}
}

// Subscription is one connection's view of the events for one person.
//
// C closes when the hub is closed, or when the subscription was dropped for
// not keeping up; either way the connection should end and the client will
// reconnect and catch up.
type Subscription struct {
	C      <-chan Event
	c      chan Event
	userID uuid.UUID
	hub    *Hub
	once   sync.Once
}

// Close detaches the subscription. Safe to call more than once.
func (s *Subscription) Close() {
	s.once.Do(func() {
		s.hub.detach(s)
		close(s.c)
	})
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

// Dispatch hands an event to every connection of every person it names.
// A connection that cannot take it is dropped rather than waited for.
func (h *Hub) Dispatch(ev Event) {
	var slow []*Subscription

	h.mu.Lock()
	for _, userID := range ev.UserIDs {
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

// Close drops every connection, for shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	var all []*Subscription
	for _, set := range h.subs {
		for s := range set {
			all = append(all, s)
		}
	}
	h.mu.Unlock()

	for _, s := range all {
		s.Close()
	}
}
