package users_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// newService returns a service over a transactional store. Every test gets its
// own transaction, so they are independent no matter what order they run in.
func newService(t *testing.T) *users.Service {
	t.Helper()
	return users.New(testutil.NewStore(t))
}

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	created, err := svc.Create(ctx, users.CreateInput{DisplayName: "Priya", Locale: "te-IN"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if created.ID == uuid.Nil {
		t.Fatal("Create returned a zero identifier")
	}
	if created.DisplayName != "Priya" {
		t.Errorf("DisplayName = %q, want Priya", created.DisplayName)
	}
	if created.Locale != "te-IN" {
		t.Errorf("Locale = %q, want te-IN", created.Locale)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("Create left timestamps unset")
	}

	fetched, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.ID != created.ID || fetched.DisplayName != created.DisplayName {
		t.Errorf("Get returned %+v, want %+v", fetched, created)
	}
}

func TestCreateDefaultsLocale(t *testing.T) {
	svc := newService(t)

	user, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Ravi"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if user.Locale != users.DefaultLocale {
		t.Errorf("Locale = %q, want the default %q", user.Locale, users.DefaultLocale)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	cases := map[string]users.CreateInput{
		"empty name":   {DisplayName: ""},
		"blank name":   {DisplayName: "   "},
		"long name":    {DisplayName: strings.Repeat("a", 81)},
		"bad locale":   {DisplayName: "Ravi", Locale: "english"},
		"name newline": {DisplayName: "Ravi\nKumar"},
	}

	svc := newService(t)
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Create(context.Background(), input); err == nil {
				t.Fatalf("Create accepted invalid input %+v", input)
			} else if domain.KindOf(err) != domain.KindInvalid {
				t.Errorf("Kind = %q, want invalid", domain.KindOf(err))
			}
		})
	}
}

func TestGetMissingUser(t *testing.T) {
	svc := newService(t)

	_, err := svc.Get(context.Background(), domain.NewID())
	if err == nil {
		t.Fatal("Get returned a user for an identifier that was never created")
	}
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("Kind = %q, want not_found", domain.KindOf(err))
	}
	if domain.CodeOf(err) != "user_not_found" {
		t.Errorf("Code = %q, want user_not_found", domain.CodeOf(err))
	}
}

// A partial update must leave untouched fields exactly as they were. Getting
// this wrong would let someone editing their name silently reset their locale.
func TestUpdateProfileChangesOnlyWhatWasSent(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewStore(t)
	svc := users.New(db)

	user := testutil.CreateUser(t, db,
		testutil.WithDisplayName("Original Name"),
		testutil.WithLocale("hi-IN"),
	)

	newName := "Changed Name"
	afterName, err := svc.UpdateProfile(ctx, user.ID, users.UpdateProfileInput{DisplayName: &newName})
	if err != nil {
		t.Fatalf("UpdateProfile returned error: %v", err)
	}
	if afterName.DisplayName != newName {
		t.Errorf("DisplayName = %q, want %q", afterName.DisplayName, newName)
	}
	if afterName.Locale != "hi-IN" {
		t.Errorf("Locale changed to %q when only the name was sent", afterName.Locale)
	}

	newLocale := "ta-IN"
	afterLocale, err := svc.UpdateProfile(ctx, user.ID, users.UpdateProfileInput{Locale: &newLocale})
	if err != nil {
		t.Fatalf("UpdateProfile returned error: %v", err)
	}
	if afterLocale.Locale != newLocale {
		t.Errorf("Locale = %q, want %q", afterLocale.Locale, newLocale)
	}
	if afterLocale.DisplayName != newName {
		t.Errorf("DisplayName changed to %q when only the locale was sent", afterLocale.DisplayName)
	}
}

// Postgres now() returns the transaction's start time, so a row created and
// updated inside one transaction would carry identical timestamps no matter
// what the UPDATE does. Backdating the row first gives a value the update has
// to move, which proves updated_at is really written.
func TestUpdateProfileTouchesUpdatedAt(t *testing.T) {
	ctx := context.Background()
	db, tx := testutil.NewStoreTx(t)
	svc := users.New(db)

	user := testutil.CreateUser(t, db)

	backdated := user.UpdatedAt.Add(-24 * time.Hour)
	if _, err := tx.Exec(ctx,
		"UPDATE users SET updated_at = $1 WHERE id = $2", backdated, user.ID); err != nil {
		t.Fatalf("backdate user: %v", err)
	}

	name := "Renamed"
	updated, err := svc.UpdateProfile(ctx, user.ID, users.UpdateProfileInput{DisplayName: &name})
	if err != nil {
		t.Fatalf("UpdateProfile returned error: %v", err)
	}

	if !updated.UpdatedAt.After(backdated) {
		t.Errorf("UpdatedAt was not advanced: backdated to %v, now %v", backdated, updated.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(user.CreatedAt) {
		t.Error("UpdateProfile changed CreatedAt")
	}
}

func TestUpdateProfileWithNoFields(t *testing.T) {
	db := testutil.NewStore(t)
	svc := users.New(db)
	user := testutil.CreateUser(t, db)

	_, err := svc.UpdateProfile(context.Background(), user.ID, users.UpdateProfileInput{})
	if err == nil {
		t.Fatal("UpdateProfile accepted an update with nothing to change")
	}
	if domain.CodeOf(err) != "no_changes" {
		t.Errorf("Code = %q, want no_changes", domain.CodeOf(err))
	}
}

func TestUpdateProfileRejectsInvalidValues(t *testing.T) {
	db := testutil.NewStore(t)
	svc := users.New(db)
	user := testutil.CreateUser(t, db, testutil.WithDisplayName("Keep Me"))

	blank := "  "
	if _, err := svc.UpdateProfile(context.Background(), user.ID,
		users.UpdateProfileInput{DisplayName: &blank}); err == nil {
		t.Fatal("UpdateProfile accepted a blank name")
	}

	// The rejected update must not have been partially applied.
	unchanged, err := svc.Get(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if unchanged.DisplayName != "Keep Me" {
		t.Errorf("a rejected update changed the stored name to %q", unchanged.DisplayName)
	}
}

func TestUpdateProfileMissingUser(t *testing.T) {
	svc := newService(t)

	name := "Ghost"
	_, err := svc.UpdateProfile(context.Background(), domain.NewID(),
		users.UpdateProfileInput{DisplayName: &name})
	if err == nil {
		t.Fatal("UpdateProfile succeeded for a user that does not exist")
	}
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("Kind = %q, want not_found", domain.KindOf(err))
	}
}
