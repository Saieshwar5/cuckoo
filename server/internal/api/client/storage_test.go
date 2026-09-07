package client_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type storageJSON struct {
	Media struct {
		UsedBytes   int64 `json:"used_bytes"`
		BudgetBytes int64 `json:"budget_bytes"`
	} `json:"media"`
	Messages struct {
		KeptDays  int     `json:"kept_days"`
		KeptSince *string `json:"kept_since"`
	} `json:"messages"`
}

// The Settings screen's line: nothing used yet, the budget, and the window.
func TestStorageReportsBudgetAndWindow(t *testing.T) {
	f := setupChat(t)
	var got storageJSON
	f.srv.AsUser(t, f.owner).Get("/v1/client/me/storage").ExpectStatus(http.StatusOK).Decode(&got)
	if got.Media.UsedBytes != 0 || got.Media.BudgetBytes != testutil.TestRetention.UserMediaBudget {
		t.Errorf("media = %+v, want 0 of %d", got.Media, testutil.TestRetention.UserMediaBudget)
	}
	if got.Messages.KeptDays != 90 || got.Messages.KeptSince == nil {
		t.Errorf("messages = %+v, want 90 days and a date", got.Messages)
	}
}

// A page of history says whether it ends because the conversation began
// there or because older messages have expired.
func TestHistoryPageSaysWhenItIsTrimmed(t *testing.T) {
	type page struct {
		NextBefore *string `json:"next_before"`
		Trimmed    bool    `json:"trimmed"`
	}
	f := setupChat(t)
	var fresh page
	f.srv.AsUser(t, f.owner).Get("/v1/client/conversations/" + f.dmID + "/messages").
		ExpectStatus(http.StatusOK).Decode(&fresh)
	if fresh.Trimmed {
		t.Error("a conversation that began today reads as trimmed")
	}

	// The same conversation seen from a hundred days later.
	later := testutil.NewServer(t, f.db, testutil.WithClock(func() time.Time { return time.Now().Add(100 * 24 * time.Hour) }))
	var old page
	later.AsUser(t, f.owner).Get("/v1/client/conversations/" + f.dmID + "/messages").
		ExpectStatus(http.StatusOK).Decode(&old)
	if !old.Trimmed {
		t.Error("a conversation older than the window does not read as trimmed")
	}
}
