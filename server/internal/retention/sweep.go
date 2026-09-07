package retention

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// batch is how many rows one statement touches. Small enough that no lock
// is held for long; large enough that a backlog clears in minutes.
const batch = 500

// Sweep applies the policy once: expired messages and their files, old
// delivery records, then anyone over their file budget.
func (s *Service) Sweep(ctx context.Context) (Report, error) {
	r := Report{DryRun: s.policy.DryRun}
	if err := s.sweepMessages(ctx, &r); err != nil {
		return r, err
	}
	if err := s.sweepDeliveries(ctx, &r); err != nil {
		return r, err
	}
	for _, kind := range []string{OwnerUser, OwnerAgent} {
		if err := s.sweepBudget(ctx, &r, kind); err != nil {
			return r, err
		}
	}
	return r, nil
}

func (s *Service) sweepMessages(ctx context.Context, r *Report) error {
	since, ok := s.Window()
	if !ok {
		return nil
	}
	if s.policy.DryRun {
		n, err := s.store.CountExpiredMessages(ctx, since)
		if err != nil {
			return domain.Internal(fmt.Errorf("count expired messages: %w", err))
		}
		r.Messages = n
		return nil
	}
	for {
		ids, err := s.store.ListExpiredMessageIDs(ctx, gen.ListExpiredMessageIDsParams{Before: since, Batch: batch})
		if err != nil {
			return domain.Internal(fmt.Errorf("list expired messages: %w", err))
		}
		if len(ids) == 0 {
			return nil
		}
		// Files first, so their keys are known before the message that
		// carried them is gone.
		files, err := s.store.DeleteMediaOfMessages(ctx, ids)
		if err != nil {
			return domain.Internal(fmt.Errorf("delete files of expired messages: %w", err))
		}
		n, err := s.store.DeleteMessagesByIDs(ctx, ids)
		if err != nil {
			return domain.Internal(fmt.Errorf("delete expired messages: %w", err))
		}
		r.Messages += n
		for _, f := range files {
			s.removeBytes(ctx, f.StorageKey, f.ThumbKey)
		}
		r.Files += len(files)
		if len(ids) < batch {
			return nil
		}
	}
}

func (s *Service) sweepDeliveries(ctx context.Context, r *Report) error {
	if s.policy.DryRun {
		return nil
	}
	n, err := s.store.DeleteFinishedDeliveries(ctx, s.now().Add(-s.policy.DeliveryAge))
	if err != nil {
		return domain.Internal(fmt.Errorf("delete finished deliveries: %w", err))
	}
	r.Deliveries = n
	return nil
}

// sweepBudget brings every owner of one kind under their budget, oldest
// files first. The message a removed file hung on keeps its words.
func (s *Service) sweepBudget(ctx context.Context, r *Report, kind string) error {
	limit := s.budget(kind)
	if limit <= 0 {
		return nil
	}
	owners, err := s.store.ListMediaOwnersOverBudget(ctx, gen.ListMediaOwnersOverBudgetParams{OwnerKind: kind, Budget: limit})
	if err != nil {
		return domain.Internal(fmt.Errorf("list %ss over budget: %w", kind, err))
	}
	r.OverBudget += len(owners)
	if s.policy.DryRun {
		return nil
	}
	for _, o := range owners {
		excess := o.TotalBytes - limit
		for excess > 0 {
			rows, err := s.store.ListClaimedMediaOldestFirst(ctx, gen.ListClaimedMediaOldestFirstParams{
				OwnerKind: kind, OwnerID: o.OwnerID, Batch: batch,
			})
			if err != nil {
				return domain.Internal(fmt.Errorf("list files of %s %s: %w", kind, o.OwnerID, err))
			}
			var ids []uuid.UUID
			for _, row := range rows {
				if excess <= 0 {
					break
				}
				ids = append(ids, row.ID)
				excess -= row.ByteSize
			}
			if len(ids) == 0 {
				break
			}
			files, err := s.store.DeleteMediaByIDs(ctx, ids)
			if err != nil {
				return domain.Internal(fmt.Errorf("delete files of %s %s: %w", kind, o.OwnerID, err))
			}
			for _, f := range files {
				s.removeBytes(ctx, f.StorageKey, f.ThumbKey)
			}
			r.Files += len(files)
		}
	}
	return nil
}

// removeBytes deletes a file and its thumbnail from storage. A file that
// will not go is logged, not fatal: the row is already gone, and a later
// sweep of orphans is the place to insist.
func (s *Service) removeBytes(ctx context.Context, key string, thumb *string) {
	if err := s.blobs.Delete(ctx, key); err != nil {
		s.log.WarnContext(ctx, "could not delete expired file", "key", key, "error", err)
	}
	if thumb != nil {
		if err := s.blobs.Delete(ctx, *thumb); err != nil {
			s.log.WarnContext(ctx, "could not delete expired thumbnail", "key", *thumb, "error", err)
		}
	}
}
