// Package retention keeps the hub a window, not an archive.
//
// The hub holds a message and its files for a period, and holds each
// uploader's files to a budget. Past either, the oldest goes first. The
// phone keeps its own copy of what it has shown, and an agent's owner
// receives every message the moment it is sent, so the hub's copy exists to
// deliver, to catch up, and to let a new device see recent history —
// nothing more.
//
// A sweep runs beside the server on a schedule. It is safe to run at any
// time and to interrupt: every step deletes what is past the line and
// nothing else, and a step that stops early is finished by the next sweep.
package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/blobs"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Owner kinds, as the media table spells them.
const (
	OwnerUser  = "user"
	OwnerAgent = "agent"
)

// Policy is what the hub keeps and for how long. A zero MessageAge keeps
// messages forever; a zero budget is no budget.
type Policy struct {
	MessageAge       time.Duration
	UserMediaBudget  int64
	AgentMediaBudget int64
	// DeliveryAge is how long a delivery record outlives its final state.
	// It is proof that an event reached a backend, not history.
	DeliveryAge time.Duration
	// DryRun counts what a sweep would remove and removes nothing.
	DryRun bool
}

// Store is what the sweeper asks of the database.
type Store interface {
	ListExpiredMessageIDs(ctx context.Context, arg gen.ListExpiredMessageIDsParams) ([]uuid.UUID, error)
	CountExpiredMessages(ctx context.Context, before time.Time) (int64, error)
	DeleteMediaOfMessages(ctx context.Context, messageIDs []uuid.UUID) ([]gen.DeleteMediaOfMessagesRow, error)
	DeleteMessagesByIDs(ctx context.Context, ids []uuid.UUID) (int64, error)
	DeleteFinishedDeliveries(ctx context.Context, before time.Time) (int64, error)
	ListMediaOwnersOverBudget(ctx context.Context, arg gen.ListMediaOwnersOverBudgetParams) ([]gen.ListMediaOwnersOverBudgetRow, error)
	ListClaimedMediaOldestFirst(ctx context.Context, arg gen.ListClaimedMediaOldestFirstParams) ([]gen.ListClaimedMediaOldestFirstRow, error)
	DeleteMediaByIDs(ctx context.Context, ids []uuid.UUID) ([]gen.DeleteMediaByIDsRow, error)
	SumClaimedMediaByOwner(ctx context.Context, arg gen.SumClaimedMediaByOwnerParams) (int64, error)
}

// Service applies a Policy.
type Service struct {
	store  Store
	blobs  blobs.Store
	policy Policy
	log    *slog.Logger
	now    func() time.Time
}

// Option configures the service.
type Option func(*Service)

// WithLogger sets where sweeps report what they did.
func WithLogger(l *slog.Logger) Option { return func(s *Service) { s.log = l } }

// WithClock replaces the clock, so a test can move time rather than wait.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// DefaultDeliveryAge is how long delivery records are kept once final.
const DefaultDeliveryAge = 7 * 24 * time.Hour

// New builds the service.
func New(st Store, b blobs.Store, p Policy, opts ...Option) *Service {
	if p.DeliveryAge <= 0 {
		p.DeliveryAge = DefaultDeliveryAge
	}
	s := &Service{store: st, blobs: b, policy: p, log: slog.Default(), now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Policy is what the service applies.
func (s *Service) Policy() Policy { return s.policy }

// Window is the moment before which no message is kept, and whether there
// is such a moment at all.
func (s *Service) Window() (time.Time, bool) {
	if s.policy.MessageAge <= 0 {
		return time.Time{}, false
	}
	return s.now().Add(-s.policy.MessageAge), true
}

// Trimmed reports whether a conversation that began at the given time may
// have had messages the hub no longer holds.
func (s *Service) Trimmed(conversationStarted time.Time) bool {
	since, ok := s.Window()
	return ok && conversationStarted.Before(since)
}

// Usage is how much of a budget one owner is using.
type Usage struct {
	UsedBytes   int64
	BudgetBytes int64
}

// Usage reports an owner's sent files against their budget.
func (s *Service) Usage(ctx context.Context, ownerKind string, ownerID uuid.UUID) (Usage, error) {
	used, err := s.store.SumClaimedMediaByOwner(ctx, gen.SumClaimedMediaByOwnerParams{OwnerKind: ownerKind, OwnerID: ownerID})
	if err != nil {
		return Usage{}, domain.Internal(fmt.Errorf("sum media of %s %s: %w", ownerKind, ownerID, err))
	}
	return Usage{UsedBytes: used, BudgetBytes: s.budget(ownerKind)}, nil
}

func (s *Service) budget(ownerKind string) int64 {
	if ownerKind == OwnerAgent {
		return s.policy.AgentMediaBudget
	}
	return s.policy.UserMediaBudget
}

// Report is what one sweep removed, or would have.
type Report struct {
	Messages   int64
	Files      int
	Deliveries int64
	OverBudget int
	DryRun     bool
}

// Run sweeps on a schedule until the context ends. The first sweep comes
// soon after start rather than a whole interval later, so a fresh deploy
// applies the policy it was started with.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if r, err := s.Sweep(ctx); err != nil {
			s.log.WarnContext(ctx, "retention sweep failed", "error", err)
		} else if r.Messages > 0 || r.Files > 0 || r.Deliveries > 0 || r.OverBudget > 0 {
			s.log.InfoContext(ctx, "retention sweep",
				"messages", r.Messages, "files", r.Files, "deliveries", r.Deliveries,
				"owners_over_budget", r.OverBudget, "dry_run", r.DryRun)
		}
		timer.Reset(every)
	}
}
