package signin_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/mail"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

var codePattern = regexp.MustCompile(`\b[0-9]{6}\b`)

type fixture struct {
	db     *store.Store
	tx     pgx.Tx
	mailer *mail.Memory
	svc    *signin.Service
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db, tx := testutil.NewStoreTx(t)
	mailer := mail.NewMemory()
	return &fixture{db: db, tx: tx, mailer: mailer, svc: signin.New(db, mailer)}
}

func (f *fixture) code(t *testing.T, email string) string {
	t.Helper()
	m, ok := f.mailer.Last(email)
	if !ok {
		t.Fatalf("no mail to %s", email)
	}
	code := codePattern.FindString(m.Text)
	if code == "" {
		t.Fatalf("no code in %q", m.Text)
	}
	return code
}

func (f *fixture) signIn(t *testing.T, email string) signin.Verified {
	t.Helper()
	if err := f.svc.Start(context.Background(), email); err != nil {
		t.Fatalf("Start: %v", err)
	}
	v, err := f.svc.Verify(context.Background(), email, f.code(t, strings.ToLower(strings.TrimSpace(email))), "Pixel 7")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	return v
}

// The whole flow: a code goes to the address, the right code opens an
// account and a session, and the session token identifies the person.
func TestSignInOpensAnAccount(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	if err := f.svc.Start(ctx, "  Priya@Example.com "); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m, ok := f.mailer.Last("priya@example.com")
	if !ok || !strings.Contains(m.Subject, "sign-in code") {
		t.Fatalf("mail = %+v, want a code sent to the lowercased address", m)
	}

	v, err := f.svc.Verify(ctx, "priya@example.com", f.code(t, "priya@example.com"), "Pixel 7")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !v.IsNew || v.User.DisplayName != "priya" || !strings.HasPrefix(v.Token, "ses_tok_") {
		t.Errorf("verified = %+v, want a new account named from the address with a session token", v)
	}
	if time.Until(v.Session.ExpiresAt) < 89*24*time.Hour {
		t.Errorf("session expires at %v, want about ninety days out", v.Session.ExpiresAt)
	}

	userID, sessionID, err := f.svc.ResolveToken(ctx, v.Token)
	if err != nil || userID != v.User.ID || sessionID != v.Session.ID {
		t.Errorf("ResolveToken = %v %v %v, want the new person's session", userID, sessionID, err)
	}

	// A code is single use.
	if _, err := f.svc.Verify(ctx, "priya@example.com", f.code(t, "priya@example.com"), ""); domain.CodeOf(err) != "invalid_code" {
		t.Errorf("reusing a code got %v, want invalid_code", err)
	}
}

// Signing in again finds the same account and signs out the earlier device.
func TestSecondSignInReusesAccountAndReplacesSession(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	first := f.signIn(t, "priya@example.com")
	second := f.signIn(t, "PRIYA@example.com")

	if second.IsNew || second.User.ID != first.User.ID {
		t.Errorf("second sign-in = new %v user %v, want the same existing account", second.IsNew, second.User.ID)
	}
	if _, _, err := f.svc.ResolveToken(ctx, first.Token); domain.CodeOf(err) != "invalid_credentials" {
		t.Errorf("first device's token still works after a second sign-in: %v", err)
	}
	if _, _, err := f.svc.ResolveToken(ctx, second.Token); err != nil {
		t.Errorf("second device's token refused: %v", err)
	}
}

func TestOnlyTheLatestCodeCounts(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	_ = f.svc.Start(ctx, "priya@example.com")
	old := f.code(t, "priya@example.com")
	_ = f.svc.Start(ctx, "priya@example.com")
	if _, err := f.svc.Verify(ctx, "priya@example.com", old, ""); domain.CodeOf(err) != "invalid_code" {
		if old != f.code(t, "priya@example.com") { // a one-in-a-million collision would be a real pass
			t.Errorf("an old code was accepted: %v", err)
		}
	}
}

func TestWrongCodesLockAfterFiveAttempts(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	_ = f.svc.Start(ctx, "priya@example.com")
	right := f.code(t, "priya@example.com")
	wrong := "000000"
	if wrong == right {
		wrong = "000001"
	}

	for i := range 5 {
		if _, err := f.svc.Verify(ctx, "priya@example.com", wrong, ""); domain.CodeOf(err) != "invalid_code" {
			t.Fatalf("attempt %d got %v, want invalid_code", i+1, err)
		}
	}
	if _, err := f.svc.Verify(ctx, "priya@example.com", right, ""); domain.CodeOf(err) != "invalid_code" {
		t.Errorf("the right code after five wrong ones got %v, want invalid_code", err)
	}
}

func TestExpiredCodeIsRefused(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	_ = f.svc.Start(ctx, "priya@example.com")
	if _, err := f.tx.Exec(ctx, "UPDATE sign_in_codes SET expires_at = now() - interval '1 minute'"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Verify(ctx, "priya@example.com", f.code(t, "priya@example.com"), ""); domain.CodeOf(err) != "invalid_code" {
		t.Errorf("expired code got %v, want invalid_code", err)
	}
}

// refusing is a Limiter that always says wait.
type refusing struct{}

func (refusing) Allow(context.Context, string, ratelimit.Policy) (time.Duration, error) {
	return time.Minute, nil
}

func TestStartRejections(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	for _, bad := range []string{"", "not an email", "a@", "@b.com", "two@@x.com", "Name <a@b.com>"} {
		if err := f.svc.Start(ctx, bad); domain.CodeOf(err) != "invalid_email" {
			t.Errorf("Start(%q) got %v, want invalid_email", bad, err)
		}
	}
	if f.mailer.Count() != 0 {
		t.Errorf("%d mails sent for invalid addresses", f.mailer.Count())
	}

	limited := signin.New(f.db, f.mailer, signin.WithLimiter(refusing{}))
	err := limited.Start(ctx, "priya@example.com")
	if e, ok := domain.AsError(err); !ok || e.Code != "too_many_codes" || e.RetryAfter != time.Minute {
		t.Errorf("limited Start got %v, want too_many_codes with a retry", err)
	}
	if _, err := f.svc.Verify(ctx, "nobody@example.com", "123456", ""); domain.CodeOf(err) != "invalid_code" {
		t.Errorf("verify with no code requested got %v, want invalid_code", err)
	}
	if _, err := f.svc.Verify(ctx, "priya@example.com", "123456", strings.Repeat("x", 81)); domain.CodeOf(err) != "invalid_device_name" {
		t.Errorf("long device name got %v, want invalid_device_name", err)
	}
}

func TestLogoutAndBadTokens(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	v := f.signIn(t, "priya@example.com")

	if err := f.svc.Logout(ctx, v.Session.ID); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, _, err := f.svc.ResolveToken(ctx, v.Token); domain.CodeOf(err) != "invalid_credentials" {
		t.Errorf("token after logout got %v, want invalid_credentials", err)
	}
	if err := f.svc.Logout(ctx, v.Session.ID); err != nil {
		t.Errorf("second Logout: %v, want nothing", err)
	}

	for _, bad := range []string{"", "bnd_sec_notasession", "ses_tok_", "ses_tok_" + strings.Repeat("a", 52)} {
		if _, _, err := f.svc.ResolveToken(ctx, bad); domain.CodeOf(err) != "invalid_credentials" {
			t.Errorf("ResolveToken(%q) got %v, want invalid_credentials", bad, err)
		}
	}
}
