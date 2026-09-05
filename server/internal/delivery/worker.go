package delivery

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

const (
	// pollInterval is how long the worker sleeps when nothing was due. A
	// second is invisible next to a backend's own response time.
	pollInterval = time.Second
	// batchSize bounds one pass. A full batch means more may be waiting, and
	// the worker goes straight round again.
	batchSize = 20
	// sendConcurrency bounds simultaneous webhook calls, so one slow backend
	// cannot hold every other agent's events behind its timeout.
	sendConcurrency = 8
	// lastErrorMaxLen keeps a backend's error page from becoming a column.
	lastErrorMaxLen = 500
)

// Options configures a Worker.
type Options struct {
	// AllowLoopback lets the worker post to this machine. Development only:
	// in production a loopback address is our own server, not a backend.
	AllowLoopback bool
	Logger        *slog.Logger
}

// Worker delivers pending events to webhook backends.
type Worker struct {
	store      *store.Store
	deliveries *Service
	agents     *agents.Service
	sender     *webhookSender
	log        *slog.Logger
}

// NewWorker builds a worker. Run it with Run, or drive it with RunOnce.
func NewWorker(st *store.Store, deliveries *Service, agentService *agents.Service, opts Options) *Worker {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		store:      st,
		deliveries: deliveries,
		agents:     agentService,
		sender:     newWebhookSender(opts.AllowLoopback),
		log:        log.With("component", "delivery"),
	}
}

// Run polls for due deliveries until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.log.Info("delivery worker started")
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		n, err := w.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			w.log.Error("delivery pass failed", "error", err)
		}
		if n >= batchSize && ctx.Err() == nil {
			continue
		}
		select {
		case <-ctx.Done():
			w.log.Info("delivery worker stopped")
			return
		case <-ticker.C:
		}
	}
}

// RunOnce fails what can no longer be delivered, then claims and attempts one
// batch of due deliveries. It returns how many were attempted.
//
// Database work happens before and after the HTTP calls, never during, so a
// batch holds no connection while waiting on a backend, and the calls
// themselves can run side by side.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	expired, err := w.store.FailExpiredDeliveries(ctx)
	if err != nil {
		return 0, fmt.Errorf("expire deliveries: %w", err)
	}
	if expired > 0 {
		w.log.Warn("deliveries expired unattempted for a day", "count", expired)
	}
	unbound, err := w.store.FailUnboundDeliveries(ctx)
	if err != nil {
		return 0, fmt.Errorf("fail unbound deliveries: %w", err)
	}
	if unbound > 0 {
		w.log.Info("deliveries failed: binding revoked", "count", unbound)
	}

	var claimed []gen.MessageDelivery
	err = w.store.WithTx(ctx, func(tx *store.Store) error {
		var err error
		claimed, err = tx.ClaimDueWebhookDeliveries(ctx, batchSize)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("claim deliveries: %w", err)
	}
	if len(claimed) == 0 {
		return 0, nil
	}

	attempts := make([]attempt, len(claimed))
	for i, row := range claimed {
		attempts[i].delivery = fromRow(row)
	}
	envelopes, err := w.deliveries.envelopes(ctx, deliveriesOf(attempts))
	if err != nil {
		return 0, err
	}
	for i := range attempts {
		attempts[i].envelope = envelopes[i]
		ep, err := w.agents.ActiveEndpoint(ctx, attempts[i].delivery.AgentID)
		if err != nil {
			return 0, err
		}
		attempts[i].endpoint = ep
	}

	w.sendAll(ctx, attempts)

	for i := range attempts {
		if err := w.record(ctx, &attempts[i]); err != nil {
			return len(attempts), err
		}
	}
	return len(attempts), nil
}

// attempt is one delivery's journey through a pass.
type attempt struct {
	delivery Delivery
	envelope events.Envelope
	endpoint *agents.Endpoint // nil when the agent has no live binding
	err      error
}

func deliveriesOf(attempts []attempt) []Delivery {
	out := make([]Delivery, len(attempts))
	for i := range attempts {
		out[i] = attempts[i].delivery
	}
	return out
}

// sendAll posts every attempt that has a webhook to post to, a bounded
// number at a time.
func (w *Worker) sendAll(ctx context.Context, attempts []attempt) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, sendConcurrency)
	for i := range attempts {
		a := &attempts[i]
		if a.endpoint == nil || a.endpoint.Mode != agents.ModeWebhook {
			continue
		}
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			a.err = w.sender.Send(ctx, a.endpoint, a.envelope)
		}()
	}
	wg.Wait()
}

// record writes the outcome of one attempt.
func (w *Worker) record(ctx context.Context, a *attempt) error {
	d := a.delivery
	switch {
	case a.endpoint == nil:
		// The binding was revoked after the message was sent.
		w.log.Info("delivery failed: no binding", "event", d.ID, "agent", d.AgentID)
		return w.markFailed(ctx, d, "no_binding")

	case a.endpoint.Mode != agents.ModeWebhook:
		// The binding changed mode under us. The lease expires and the
		// claim's own filter will leave it for the right transport.
		return nil

	case a.err == nil:
		if err := w.store.MarkDelivered(ctx, d.ID); err != nil {
			return fmt.Errorf("mark %s delivered: %w", d.ID, err)
		}
		w.log.Debug("delivered", "event", d.ID, "agent", d.AgentID, "attempt", d.Attempts)
		w.tickChanged(ctx, d)
		return w.agents.RecordDeliverySuccess(ctx, a.endpoint.BindingID)
	}

	if err := w.agents.RecordDeliveryFailure(ctx, a.endpoint.BindingID); err != nil {
		return err
	}
	reason := truncate(a.err.Error(), lastErrorMaxLen)
	next, ok := nextAttempt(d.Attempts, d.CreatedAt, time.Now())
	if !ok {
		w.log.Warn("delivery failed: retries exhausted",
			"event", d.ID, "agent", d.AgentID, "attempts", d.Attempts, "error", reason)
		return w.markFailed(ctx, d, "exhausted: "+reason)
	}
	w.log.Warn("delivery attempt failed",
		"event", d.ID, "agent", d.AgentID, "attempt", d.Attempts, "retry_at", next, "error", reason)
	if err := w.store.ScheduleRetry(ctx, gen.ScheduleRetryParams{
		ID: d.ID, NextAttemptAt: next, LastError: &reason,
	}); err != nil {
		return fmt.Errorf("schedule retry of %s: %w", d.ID, err)
	}
	return nil
}

func (w *Worker) markFailed(ctx context.Context, d Delivery, reason string) error {
	reason = truncate(reason, lastErrorMaxLen)
	if err := w.store.MarkFailed(ctx, gen.MarkFailedParams{ID: d.ID, LastError: &reason}); err != nil {
		return fmt.Errorf("mark %s failed: %w", d.ID, err)
	}
	w.tickChanged(ctx, d)
	return nil
}

// tickChanged tells the sender's devices the tick mark moved. It cannot fail
// the pass: the outcome is recorded, and a device catches up on reconnect.
func (w *Worker) tickChanged(ctx context.Context, d Delivery) {
	if err := w.deliveries.conversations.DeliveryChanged(ctx, d.MessageID); err != nil {
		w.log.Warn("could not announce delivery change", "event", d.ID, "error", err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
