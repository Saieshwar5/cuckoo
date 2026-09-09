package realtime

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Presence answers one question: is this person looking at the app right now?
//
// It exists because push notifications are only welcome when they are not.
// A message somebody is already watching arrive should not also buzz their
// pocket, and getting that wrong is not a small annoyance — it is the reason
// people turn notifications off and never turn them back on.
//
// The Hub knows who is connected, but only to *this* process. With one
// instance that is the whole truth; with two it is half of it, and the half it
// is missing is the half that produces spurious notifications. So presence
// lives in Redis, which every instance already shares, rather than in a map
// that is only ever right by accident.
//
// Nothing here is durable. A restart forgets who was connected, and every
// connection re-announces itself within a heartbeat.
type Presence interface {
	// Arrived marks a person as connected until the next heartbeat lapses.
	Arrived(ctx context.Context, userID uuid.UUID) error
	// Left marks them gone. Best effort: the key expires anyway.
	Left(ctx context.Context, userID uuid.UUID) error
	// Watching reports which of these people have a live connection.
	Watching(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// presenceTTL is how long a connection is believed in without saying so again.
// Long enough that a heartbeat is cheap, short enough that a process killed
// mid-connection stops suppressing somebody's notifications within a minute.
const presenceTTL = 90 * time.Second

// HeartbeatInterval is how often a live connection should re-announce itself.
// Comfortably inside the TTL, so one missed beat is not a disappearance.
const HeartbeatInterval = 30 * time.Second

// RedisPresence keeps presence in Redis, shared by every instance.
type RedisPresence struct {
	client *redis.Client
	prefix string
}

func NewRedisPresence(client *redis.Client, prefix string) *RedisPresence {
	if prefix == "" {
		prefix = "presence"
	}
	return &RedisPresence{client: client, prefix: prefix}
}

func (p *RedisPresence) key(userID uuid.UUID) string {
	return fmt.Sprintf("%s:%s", p.prefix, userID)
}

func (p *RedisPresence) Arrived(ctx context.Context, userID uuid.UUID) error {
	if err := p.client.Set(ctx, p.key(userID), 1, presenceTTL).Err(); err != nil {
		return fmt.Errorf("mark present: %w", err)
	}
	return nil
}

func (p *RedisPresence) Left(ctx context.Context, userID uuid.UUID) error {
	if err := p.client.Del(ctx, p.key(userID)).Err(); err != nil {
		return fmt.Errorf("mark absent: %w", err)
	}
	return nil
}

func (p *RedisPresence) Watching(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		keys = append(keys, p.key(id))
	}
	values, err := p.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("read presence: %w", err)
	}
	for i, id := range userIDs {
		out[id] = i < len(values) && values[i] != nil
	}
	return out, nil
}

// NoPresence believes nobody is connected, so everything is notified. What a
// hub with no Redis behind it falls back to: over-notifying is a worse app,
// but under-notifying is a broken one.
type NoPresence struct{}

func (NoPresence) Arrived(context.Context, uuid.UUID) error { return nil }
func (NoPresence) Left(context.Context, uuid.UUID) error    { return nil }
func (NoPresence) Watching(_ context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(userIDs))
	for _, id := range userIDs {
		out[id] = false
	}
	return out, nil
}
