// Package mail sends the few emails the hub needs to send: today, sign-in
// codes.
//
// The interface is small on purpose. Development prints to the log, tests
// keep messages in memory, and a real provider is one more implementation,
// chosen by configuration on the day the hub is hosted.
package mail

import (
	"context"
	"log/slog"
	"sync"
)

// Message is one email.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Mailer delivers messages.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// Console writes messages to the log instead of sending them. For
// development, where the person reading the log is the person signing in.
type Console struct {
	log *slog.Logger
}

// NewConsole builds a console mailer.
func NewConsole(log *slog.Logger) *Console {
	if log == nil {
		log = slog.Default()
	}
	return &Console{log: log}
}

// Send implements Mailer.
func (c *Console) Send(ctx context.Context, m Message) error {
	c.log.InfoContext(ctx, "mail (console mode, not sent)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

// Memory keeps every message, for tests that need to read a code back.
type Memory struct {
	mu   sync.Mutex
	sent []Message
}

// NewMemory builds an in-memory mailer.
func NewMemory() *Memory { return &Memory{} }

// Send implements Mailer.
func (m *Memory) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

// Last returns the most recent message to an address.
func (m *Memory) Last(to string) (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].To == to {
			return m.sent[i], true
		}
	}
	return Message{}, false
}

// Count returns how many messages were sent.
func (m *Memory) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}
