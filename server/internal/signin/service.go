package signin

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/mail"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// startPolicy bounds codes per address: five, then one every two minutes.
// This is what stops the hub being used to fill someone's inbox.
var startPolicy = ratelimit.Policy{Rate: 1.0 / 120, Burst: 5}

// Service holds the rules for signing in.
type Service struct {
	store   *store.Store
	mailer  mail.Mailer
	limiter ratelimit.Limiter
	// welcome runs for every account that did not exist before, inside the
	// transaction that creates it; what it returns runs after the commit.
	welcome WelcomeFunc
}

// WelcomeFunc gives a new account its first contact. See pairing.Welcomer.
type WelcomeFunc func(ctx context.Context, tx *store.Store, userID uuid.UUID) (after func(), err error)

// Option configures a Service.
type Option func(*Service)

// WithLimiter bounds how often codes may be requested for one address.
func WithLimiter(l ratelimit.Limiter) Option {
	return func(s *Service) { s.limiter = l }
}

// WithWelcome sets what a new account is given on arrival. Nil means nothing.
func WithWelcome(fn WelcomeFunc) Option {
	return func(s *Service) { s.welcome = fn }
}

// New builds the service.
func New(st *store.Store, mailer mail.Mailer, opts ...Option) *Service {
	s := &Service{store: st, mailer: mailer, limiter: ratelimit.Unlimited{}}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start sends a code to an address. It says nothing about whether an account
// exists: the answer is the same either way, and the code goes to the
// address, so only its owner learns anything.
func (s *Service) Start(ctx context.Context, rawEmail string) error {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return err
	}

	wait, err := s.limiter.Allow(ctx, "signin:"+email, startPolicy)
	if err != nil {
		return domain.Internal(fmt.Errorf("rate limit: %w", err))
	}
	if wait > 0 {
		return domain.RateLimited("too_many_codes",
			"Too many codes were requested for this address. Try again later.", wait)
	}

	id := domain.NewID()
	code, err := newCode()
	if err != nil {
		return domain.Internal(err)
	}
	if _, err := s.store.CreateSignInCode(ctx, gen.CreateSignInCodeParams{
		ID: id, Email: email, CodeHash: hashCode(id, code), ExpiresAt: time.Now().Add(codeTTL),
	}); err != nil {
		return domain.Internal(fmt.Errorf("create sign-in code: %w", err))
	}

	if err := s.mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: "Your Cuckoo sign-in code",
		Text:    fmt.Sprintf("Your sign-in code is %s. It expires in %d minutes.", code, int(codeTTL.Minutes())),
	}); err != nil {
		return domain.Internal(fmt.Errorf("send sign-in code: %w", err))
	}
	return nil
}

// Verify checks a code and signs the person in. A new address becomes a new
// account. Every other device of that person is signed out.
func (s *Service) Verify(ctx context.Context, rawEmail, code, deviceName string) (Verified, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return Verified{}, err
	}
	device, err := validateDeviceName(deviceName)
	if err != nil {
		return Verified{}, err
	}

	row, err := s.store.GetLatestSignInCode(ctx, email)
	if err != nil {
		if store.IsNoRows(err) {
			return Verified{}, errInvalidCode()
		}
		return Verified{}, domain.Internal(fmt.Errorf("get sign-in code: %w", err))
	}
	if row.UsedAt != nil || time.Now().After(row.ExpiresAt) {
		return Verified{}, errInvalidCode()
	}
	// Count the attempt before checking it, so a guess that fails still
	// costs one, and so the sixth try is refused whatever it says.
	attempts, err := s.store.CountSignInAttempt(ctx, row.ID)
	if err != nil {
		return Verified{}, domain.Internal(fmt.Errorf("count sign-in attempt: %w", err))
	}
	if attempts > codeAttemptsMax {
		return Verified{}, errInvalidCode()
	}
	if subtle.ConstantTimeCompare(hashCode(row.ID, strings.TrimSpace(code)), row.CodeHash) != 1 {
		return Verified{}, errInvalidCode()
	}

	var (
		out   Verified
		token string
	)
	var after func()
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		if err := tx.MarkSignInCodeUsed(ctx, row.ID); err != nil {
			return domain.Internal(fmt.Errorf("mark code used: %w", err))
		}

		userID, isNew, err := s.accountFor(ctx, tx, email)
		if err != nil {
			return err
		}
		if isNew && s.welcome != nil {
			if after, err = s.welcome(ctx, tx, userID); err != nil {
				return err
			}
		}

		var hash []byte
		token, hash = domain.NewSecret(domain.PrefixSessionToken)
		sess, err := tx.CreateSession(ctx, gen.CreateSessionParams{
			ID: domain.NewID(), UserID: userID, TokenHash: hash, DeviceName: device,
			ExpiresAt: time.Now().Add(sessionTTL),
		})
		if err != nil {
			return domain.Internal(fmt.Errorf("create session: %w", err))
		}
		// A person may hold several devices; past the limit the one that has
		// gone longest without being used is signed out to make room.
		if _, err := tx.RevokeOldestUserSessions(ctx, gen.RevokeOldestUserSessionsParams{
			UserID: userID, Keep: devicesMax,
		}); err != nil {
			return domain.Internal(fmt.Errorf("hold %s to %d devices: %w", userID, devicesMax, err))
		}
		user, err := users.New(tx).Get(ctx, userID)
		if err != nil {
			return err
		}
		out = Verified{
			User:    user,
			Session: Session{ID: sess.ID, UserID: sess.UserID, ExpiresAt: sess.ExpiresAt},
			IsNew:   isNew,
		}
		return nil
	})
	if err != nil {
		if _, classified := domain.AsError(err); classified {
			return Verified{}, err
		}
		return Verified{}, domain.Internal(fmt.Errorf("sign in: %w", err))
	}
	out.Token = token
	if after != nil {
		after()
	}
	return out, nil
}

// accountFor finds the account behind a proven address, or opens one.
func (s *Service) accountFor(ctx context.Context, tx *store.Store, email string) (uuid.UUID, bool, error) {
	identity, err := tx.GetIdentity(ctx, gen.GetIdentityParams{Kind: "email", Value: email})
	if err == nil {
		return identity.UserID, false, nil
	}
	if !store.IsNoRows(err) {
		return uuid.Nil, false, domain.Internal(fmt.Errorf("get identity: %w", err))
	}

	user, err := users.New(tx).Create(ctx, users.CreateInput{DisplayName: displayNameFor(email)})
	if err != nil {
		return uuid.Nil, false, err
	}
	if _, err := tx.CreateIdentity(ctx, gen.CreateIdentityParams{
		ID: domain.NewID(), UserID: user.ID, Kind: "email", Value: email,
	}); err != nil {
		return uuid.Nil, false, domain.Internal(fmt.Errorf("create identity: %w", err))
	}
	return user.ID, true, nil
}

// ResolveToken identifies the person and session a bearer token belongs to.
// This runs on every request from the app, so it is one indexed lookup.
func (s *Service) ResolveToken(ctx context.Context, token string) (userID, sessionID uuid.UUID, err error) {
	if !strings.HasPrefix(token, domain.PrefixSessionToken+"_") {
		return uuid.Nil, uuid.Nil, errBadToken()
	}
	row, err := s.store.AuthenticateSession(ctx, domain.HashSecret(token))
	if err != nil {
		if store.IsNoRows(err) {
			return uuid.Nil, uuid.Nil, errBadToken()
		}
		return uuid.Nil, uuid.Nil, domain.Internal(fmt.Errorf("authenticate session: %w", err))
	}
	s.touch(ctx, row.ID, row.LastSeenAt)
	return row.UserID, row.ID, nil
}

// touch records that a session is in use, at most once an interval. A
// failure here is not the request's problem: a device list an hour stale is
// worth less than the request it would fail.
func (s *Service) touch(ctx context.Context, sessionID uuid.UUID, lastSeen *time.Time) {
	if lastSeen != nil && time.Since(*lastSeen) < seenInterval {
		return
	}
	_ = s.store.TouchSession(ctx, sessionID)
}

// Devices lists what a person is signed in on, newest first, marking the one
// asking.
func (s *Service) Devices(ctx context.Context, userID, currentSessionID uuid.UUID) ([]Device, error) {
	rows, err := s.store.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list devices of %s: %w", userID, err))
	}
	out := make([]Device, 0, len(rows))
	for _, r := range rows {
		out = append(out, Device{
			ID: r.ID, Name: r.DeviceName, LastSeen: r.LastSeenAt,
			CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, Current: r.ID == currentSessionID,
		})
	}
	return out, nil
}

// SignOutDevice ends one of a person's devices. One that is already over, or
// was never theirs, is not found rather than a silent success.
func (s *Service) SignOutDevice(ctx context.Context, userID, sessionID uuid.UUID) error {
	n, err := s.store.RevokeUserSession(ctx, gen.RevokeUserSessionParams{ID: sessionID, UserID: userID})
	if err != nil {
		return domain.Internal(fmt.Errorf("revoke session %s: %w", sessionID, err))
	}
	if n == 0 {
		return domain.NotFound("device_not_found", "That device is already signed out.")
	}
	return nil
}

// SignOutOthers ends every device but the one asking: what a person taps
// when a phone is lost.
func (s *Service) SignOutOthers(ctx context.Context, userID, keepSessionID uuid.UUID) (int, error) {
	n, err := s.store.RevokeOtherUserSessions(ctx, gen.RevokeOtherUserSessionsParams{UserID: userID, Keep: keepSessionID})
	if err != nil {
		return 0, domain.Internal(fmt.Errorf("revoke other sessions of %s: %w", userID, err))
	}
	return int(n), nil
}

// Logout ends a session. Ending one that is already over is not an error.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	if _, err := s.store.RevokeSession(ctx, sessionID); err != nil {
		return domain.Internal(fmt.Errorf("revoke session %s: %w", sessionID, err))
	}
	return nil
}

func errInvalidCode() error {
	return domain.Unauthorized("invalid_code", "That code is not valid. Request a new one.")
}

func errBadToken() error {
	return domain.Unauthorized("invalid_credentials",
		"That session is not valid. It may have expired, or been signed out from another device.")
}
