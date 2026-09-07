package client_test

import (
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type deviceJSON struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	LastSeen  *string `json:"last_seen_at"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt string  `json:"expires_at"`
	Current   bool    `json:"current"`
}

type devicesJSON struct {
	Devices []deviceJSON `json:"devices"`
}

// onDevice signs a person in on one more device and returns its token.
func onDevice(t *testing.T, srv *testutil.Server, email, device string) string {
	t.Helper()
	anon := srv.Anonymous(t)
	anon.Post("/v1/auth/email/start", map[string]any{"email": email}).ExpectStatus(http.StatusNoContent)
	var out struct {
		Token string `json:"token"`
	}
	anon.Post("/v1/auth/email/verify", map[string]any{
		"email": email, "code": srv.LastCode(t, email), "device_name": device,
	}).ExpectStatus(http.StatusOK).Decode(&out)
	if out.Token == "" {
		t.Fatal("verify returned no token")
	}
	return out.Token
}

// Two devices both stay signed in, each sees both, and each knows which one
// it is. This is the rule that changed: signing in used to end every other
// session.
func TestSigningInTwiceKeepsBothDevices(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db)
	const email = "priya@example.test"

	phone := onDevice(t, srv, email, "phone")
	laptop := onDevice(t, srv, email, "laptop")

	// The first device still works.
	srv.AsSession(t, phone).Get("/v1/client/me").ExpectStatus(http.StatusOK)

	var seen devicesJSON
	srv.AsSession(t, laptop).Get("/v1/client/devices").ExpectStatus(http.StatusOK).Decode(&seen)
	if len(seen.Devices) != 2 {
		t.Fatalf("got %d devices, want 2: %+v", len(seen.Devices), seen.Devices)
	}
	var current, names int
	for _, d := range seen.Devices {
		if d.Current {
			current++
			if d.Name != "laptop" {
				t.Errorf("the current device is %q, want laptop", d.Name)
			}
		}
		if d.Name == "phone" || d.Name == "laptop" {
			names++
		}
	}
	if current != 1 || names != 2 {
		t.Errorf("current=%d named=%d, want exactly one current and both names", current, names)
	}
}

// A person signs out a device they no longer hold, and its token stops
// working immediately.
func TestSigningOutOneDevice(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db)
	const email = "ravi@example.test"

	phone := onDevice(t, srv, email, "phone")
	laptop := onDevice(t, srv, email, "laptop")

	var seen devicesJSON
	srv.AsSession(t, laptop).Get("/v1/client/devices").ExpectStatus(http.StatusOK).Decode(&seen)
	var phoneID string
	for _, d := range seen.Devices {
		if d.Name == "phone" {
			phoneID = d.ID
		}
	}
	if phoneID == "" {
		t.Fatal("the phone is not in the list")
	}

	srv.AsSession(t, laptop).Delete("/v1/client/devices/" + phoneID).ExpectStatus(http.StatusNoContent)
	srv.AsSession(t, phone).Get("/v1/client/me").ExpectStatus(http.StatusUnauthorized)
	srv.AsSession(t, laptop).Get("/v1/client/me").ExpectStatus(http.StatusOK)

	// Signing out a device that is already out says so rather than
	// pretending it worked.
	srv.AsSession(t, laptop).Delete("/v1/client/devices/" + phoneID).ExpectStatus(http.StatusNotFound)
}

// One device may not sign out another person's.
func TestADeviceOfSomebodyElseIsNotFound(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db)
	mine := onDevice(t, srv, "mine@example.test", "phone")
	theirs := onDevice(t, srv, "theirs@example.test", "phone")

	var seen devicesJSON
	srv.AsSession(t, theirs).Get("/v1/client/devices").ExpectStatus(http.StatusOK).Decode(&seen)
	srv.AsSession(t, mine).Delete("/v1/client/devices/" + seen.Devices[0].ID).
		ExpectStatus(http.StatusNotFound)
	srv.AsSession(t, theirs).Get("/v1/client/me").ExpectStatus(http.StatusOK)
}

// What a person taps when a phone is lost: everything but this one.
func TestSigningOutEveryOtherDevice(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db)
	const email = "lost@example.test"

	first := onDevice(t, srv, email, "old phone")
	second := onDevice(t, srv, email, "tablet")
	third := onDevice(t, srv, email, "new phone")

	srv.AsSession(t, third).Delete("/v1/client/devices").ExpectStatus(http.StatusNoContent)
	srv.AsSession(t, first).Get("/v1/client/me").ExpectStatus(http.StatusUnauthorized)
	srv.AsSession(t, second).Get("/v1/client/me").ExpectStatus(http.StatusUnauthorized)
	srv.AsSession(t, third).Get("/v1/client/me").ExpectStatus(http.StatusOK)

	var left devicesJSON
	srv.AsSession(t, third).Get("/v1/client/devices").ExpectStatus(http.StatusOK).Decode(&left)
	if len(left.Devices) != 1 || !left.Devices[0].Current {
		t.Errorf("after signing out the others: %+v", left.Devices)
	}
}
