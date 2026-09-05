package conversations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// StreamStore keeps the growing text of streaming messages in Redis.
//
// Each stream is a hash under a key that names the agent as well as the
// message, so an agent can only ever touch its own; and every live stream
// is a member of one sorted set scored by its last activity, which is how
// the sweeper finds the ones that went quiet.
type StreamStore struct {
	client *redis.Client
	prefix string
}

// NewStreamStore builds a store. Every hub instance sharing a Redis uses the
// same prefix; tests use one of their own.
func NewStreamStore(client *redis.Client, prefix string) *StreamStore {
	return &StreamStore{client: client, prefix: prefix}
}

const (
	// streamMaxBytes bounds a stream's buffer. It is the text limit in
	// characters times the widest UTF-8 character; the final text is also
	// clamped to the character limit when the stream finishes.
	streamMaxBytes = textMaxLen * 4
	// streamTTL is how long an untouched buffer survives. The sweeper
	// finishes an idle stream long before this; the TTL is the safety net
	// under the safety net.
	streamTTL = 10 * time.Minute
)

var (
	errNotStreaming  = errors.New("stream: not streaming")
	errStreamTooLong = errors.New("stream: too long")
)

// streamRef names one live stream.
type streamRef struct {
	AgentID   uuid.UUID
	MessageID uuid.UUID
}

func (s *StreamStore) key(agentID, messageID uuid.UUID) string {
	return s.prefix + "stream:" + agentID.String() + ":" + messageID.String()
}

func (s *StreamStore) activeKey() string { return s.prefix + "streams" }

func memberOf(agentID, messageID uuid.UUID) string {
	return agentID.String() + ":" + messageID.String()
}

func parseMember(m string) (streamRef, bool) {
	a, b, ok := strings.Cut(m, ":")
	if !ok {
		return streamRef{}, false
	}
	agentID, err := uuid.Parse(a)
	if err != nil {
		return streamRef{}, false
	}
	messageID, err := uuid.Parse(b)
	if err != nil {
		return streamRef{}, false
	}
	return streamRef{AgentID: agentID, MessageID: messageID}, true
}

// open creates an empty buffer. The conversation and its people are kept
// with it, so a delta can be announced without a database read.
func (s *StreamStore) open(ctx context.Context, ref streamRef, conversationID uuid.UUID, userIDs []uuid.UUID) error {
	people := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		people = append(people, id.String())
	}
	key := s.key(ref.AgentID, ref.MessageID)
	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, key, "text", "", "conv", conversationID.String(), "users", strings.Join(people, ","))
	pipe.PExpire(ctx, key, streamTTL)
	pipe.ZAdd(ctx, s.activeKey(), redis.Z{Score: float64(time.Now().UnixMilli()), Member: memberOf(ref.AgentID, ref.MessageID)})
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	return nil
}

// appendScript adds a delta if the stream is live and has room, refreshes
// its lifetime and activity, and returns where to announce it. One script,
// so two instances of the hub appending at once cannot lose a piece.
var appendScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return {-1, '', ''}
end
local text = redis.call('HGET', KEYS[1], 'text')
if text == false then text = '' end
if #text + #ARGV[1] > tonumber(ARGV[2]) then
  return {-2, '', ''}
end
redis.call('HSET', KEYS[1], 'text', text .. ARGV[1])
redis.call('PEXPIRE', KEYS[1], ARGV[3])
redis.call('ZADD', KEYS[2], ARGV[4], ARGV[5])
return {1, redis.call('HGET', KEYS[1], 'conv'), redis.call('HGET', KEYS[1], 'users')}
`)

// append adds text to a live stream and returns the conversation and the
// people to tell.
func (s *StreamStore) append(ctx context.Context, ref streamRef, delta string) (uuid.UUID, []uuid.UUID, error) {
	res, err := appendScript.Run(ctx, s.client,
		[]string{s.key(ref.AgentID, ref.MessageID), s.activeKey()},
		delta, streamMaxBytes, streamTTL.Milliseconds(), time.Now().UnixMilli(), memberOf(ref.AgentID, ref.MessageID),
	).Slice()
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("append to stream: %w", err)
	}
	if len(res) != 3 {
		return uuid.Nil, nil, fmt.Errorf("append to stream: unexpected reply %v", res)
	}
	switch code, _ := res[0].(int64); code {
	case -1:
		return uuid.Nil, nil, errNotStreaming
	case -2:
		return uuid.Nil, nil, errStreamTooLong
	}
	conv, _ := res[1].(string)
	conversationID, err := uuid.Parse(conv)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("append to stream: bad conversation %q", conv)
	}
	people, _ := res[2].(string)
	var userIDs []uuid.UUID
	for _, p := range strings.Split(people, ",") {
		if id, err := uuid.Parse(p); err == nil {
			userIDs = append(userIDs, id)
		}
	}
	return conversationID, userIDs, nil
}

// text returns the buffered text so far, and whether the stream exists.
func (s *StreamStore) text(ctx context.Context, ref streamRef) (string, bool, error) {
	text, err := s.client.HGet(ctx, s.key(ref.AgentID, ref.MessageID), "text").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read stream: %w", err)
	}
	return text, true, nil
}

// texts reads several streams at once, for history that includes messages
// still being written. Missing streams read as empty.
func (s *StreamStore) texts(ctx context.Context, refs []streamRef) (map[uuid.UUID]string, error) {
	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(refs))
	for i, ref := range refs {
		cmds[i] = pipe.HGet(ctx, s.key(ref.AgentID, ref.MessageID), "text")
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("read streams: %w", err)
	}
	out := make(map[uuid.UUID]string, len(refs))
	for i, ref := range refs {
		if text, err := cmds[i].Result(); err == nil {
			out[ref.MessageID] = text
		}
	}
	return out, nil
}

// remove forgets a stream once it is finished.
func (s *StreamStore) remove(ctx context.Context, ref streamRef) error {
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.key(ref.AgentID, ref.MessageID))
	pipe.ZRem(ctx, s.activeKey(), memberOf(ref.AgentID, ref.MessageID))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("remove stream: %w", err)
	}
	return nil
}

// idle lists streams with no activity for at least idleFor.
func (s *StreamStore) idle(ctx context.Context, idleFor time.Duration) ([]streamRef, error) {
	cutoff := time.Now().Add(-idleFor).UnixMilli()
	members, err := s.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: s.activeKey(), Start: "-inf", Stop: fmt.Sprintf("%d", cutoff), ByScore: true, Count: 100,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("list idle streams: %w", err)
	}
	refs := make([]streamRef, 0, len(members))
	for _, m := range members {
		if ref, ok := parseMember(m); ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
