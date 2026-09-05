// Package realtime carries events from wherever they happen to whichever
// connections should hear them, across every instance of the hub.
//
// An Event names the people it is for. The publisher decides that from the
// database at the moment the event is created, so a connection needs no
// membership state of its own and can never be out of date about which
// conversations its person is in. Events travel over a Redis channel; with
// one instance that is a loopback, and with two it is the reason a socket on
// one instance hears a reply that arrived on the other.
//
// Nothing here is durable. A connection that was away missed events and
// catches up from the database, which is the record; the socket is only the
// hint that something changed.
package realtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Event is something a connection should hear about now. Recipients are
// the identifiers of the principals it is for: people, whose devices render
// it, or agents, whose sockets treat it as a nudge to read the outbox.
type Event struct {
	Type       string          `json:"type"`
	Recipients []uuid.UUID     `json:"recipients"`
	Payload    json.RawMessage `json:"payload"`
}

// NewEvent encodes a payload into an Event.
func NewEvent(eventType string, recipients []uuid.UUID, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("encode %s event: %w", eventType, err)
	}
	return Event{Type: eventType, Recipients: recipients, Payload: raw}, nil
}

// Publisher is what the business layer holds: a way to announce an event.
type Publisher interface {
	Publish(ctx context.Context, ev Event) error
}

// Discard is a Publisher that drops everything, for callers with nobody
// listening: agent creation, tests of other things.
type Discard struct{}

// Publish implements Publisher.
func (Discard) Publish(context.Context, Event) error { return nil }

// Bus is a Publisher that can also be listened to.
type Bus interface {
	Publisher
	// Subscribe returns a channel of events. It returns only once the
	// subscription is live, so anything published after it returns will be
	// received; the channel closes when ctx ends.
	Subscribe(ctx context.Context) (<-chan Event, error)
}

// RedisBus is a Bus over one Redis pub/sub channel.
type RedisBus struct {
	client  *redis.Client
	channel string
}

// NewRedisBus builds a bus on the named channel. Every hub instance sharing a
// Redis uses the same name; tests use a name of their own.
func NewRedisBus(client *redis.Client, channel string) *RedisBus {
	return &RedisBus{client: client, channel: channel}
}

// Publish implements Publisher.
func (b *RedisBus) Publish(ctx context.Context, ev Event) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if err := b.client.Publish(ctx, b.channel, raw).Err(); err != nil {
		return fmt.Errorf("publish %s: %w", ev.Type, err)
	}
	return nil
}

// Subscribe implements Bus.
func (b *RedisBus) Subscribe(ctx context.Context) (<-chan Event, error) {
	ps := b.client.Subscribe(ctx, b.channel)
	// The first receive is the server confirming the subscription. Waiting
	// for it is what lets a caller publish immediately afterwards and know
	// the event will arrive.
	if _, err := ps.Receive(ctx); err != nil {
		_ = ps.Close()
		return nil, fmt.Errorf("subscribe %s: %w", b.channel, err)
	}

	out := make(chan Event, 256)
	go func() {
		defer close(out)
		defer func() { _ = ps.Close() }()
		in := ps.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-in:
				if !ok {
					return
				}
				var ev Event
				if err := json.Unmarshal([]byte(m.Payload), &ev); err != nil {
					// A malformed event is a bug in a publisher, not a
					// reason to stop listening for the rest.
					continue
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
