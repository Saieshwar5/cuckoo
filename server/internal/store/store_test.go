package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// The test store is one transaction, and a transaction is one connection. A
// server under test does database work from several goroutines at once —
// socket handlers, the delivery worker — so the store must serialise them
// rather than let pgx refuse with "conn busy".
func TestTransactionalStoreIsSafeForConcurrentUse(t *testing.T) {
	db := testutil.NewStore(t)
	ctx := context.Background()
	user := testutil.CreateUser(t, db)

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				if _, err := db.GetUser(ctx, user.ID); err != nil {
					errs <- err
					return
				}
				if _, err := db.ListAgentsByOwner(ctx, user.ID); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent query failed: %v", err)
	}
}
