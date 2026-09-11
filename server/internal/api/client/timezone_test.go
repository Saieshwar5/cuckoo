package client_test

import (
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// The phone says where its clock is; the account keeps it, and a nonsense
// zone is refused rather than stored.
func TestPhoneReportsItsTimezone(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db)
	me := srv.AsUser(t, testutil.CreateUser(t, db))

	var env struct {
		User struct {
			Timezone string `json:"timezone"`
		} `json:"user"`
	}
	me.Get("/v1/client/me").ExpectStatus(http.StatusOK).Decode(&env)
	if env.User.Timezone != "" {
		t.Errorf("a new account's zone = %q, want empty until a phone says", env.User.Timezone)
	}
	me.Patch("/v1/client/me", map[string]any{"timezone": "Asia/Kolkata"}).ExpectStatus(http.StatusOK).Decode(&env)
	if env.User.Timezone != "Asia/Kolkata" {
		t.Errorf("zone = %q", env.User.Timezone)
	}
	me.Patch("/v1/client/me", map[string]any{"timezone": "Mars/Olympus"}).ExpectError(http.StatusUnprocessableEntity, "invalid_timezone")
}
