package apikeys_test

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/apikeys"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func code(t *testing.T, err error) string {
	t.Helper()
	derr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("not a classified error: %v", err)
	}
	return derr.Code
}

// A key is issued once, works, is listed without its secret, and stops
// working the moment it is revoked.
func TestKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewStore(t)
	svc := apikeys.New(db)
	owner := testutil.CreateUser(t, db)

	key, plaintext, err := svc.Create(ctx, owner.ID, "  SBI website  ")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if key.Name != "SBI website" {
		t.Errorf("name = %q, want it trimmed", key.Name)
	}
	if !strings.HasPrefix(plaintext, "mgt_tok_") {
		t.Errorf("key %q lacks the mgt_tok_ prefix", plaintext)
	}
	if key.RevokedAt != nil || key.LastUsedAt != nil {
		t.Errorf("new key = %+v, want live and unused", key)
	}

	got, err := svc.Resolve(ctx, plaintext)
	if err != nil || got != owner.ID {
		t.Fatalf("Resolve = %v, %v; want %v", got, err, owner.ID)
	}

	list, err := svc.ListMine(ctx, owner.ID)
	if err != nil || len(list) != 1 || list[0].ID != key.ID {
		t.Fatalf("ListMine = %+v, %v", list, err)
	}
	// Using it is recorded, so a forgotten key is visible.
	if list[0].LastUsedAt == nil {
		t.Error("last used not recorded")
	}

	if err := svc.Revoke(ctx, owner.ID, key.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := svc.Resolve(ctx, plaintext); code(t, err) != "invalid_credentials" {
		t.Errorf("revoked key still resolves: %v", err)
	}
	if err := svc.Revoke(ctx, owner.ID, key.ID); code(t, err) != "key_not_found" {
		t.Errorf("second revoke = %v, want key_not_found", err)
	}
	// It stays on the list: knowing a key existed and ended is part of
	// knowing who could reach the account.
	list, _ = svc.ListMine(ctx, owner.ID)
	if len(list) != 1 || list[0].RevokedAt == nil {
		t.Errorf("after revoke, list = %+v", list)
	}
}

// The key exists nowhere but in the response that created it.
func TestKeyIsStoredOnlyAsHash(t *testing.T) {
	ctx := context.Background()
	db, tx := testutil.NewStoreTx(t)
	svc := apikeys.New(db)
	owner := testutil.CreateUser(t, db)

	_, plaintext, err := svc.Create(ctx, owner.ID, "deploy script")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var stored []byte
	if err := tx.QueryRow(ctx, "SELECT key_hash FROM api_keys WHERE user_id = $1", owner.ID).Scan(&stored); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	want := sha256.Sum256([]byte(plaintext))
	if string(stored) != string(want[:]) {
		t.Error("stored value is not SHA-256 of the key")
	}
}

func TestKeysAreRefusedAndScoped(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewStore(t)
	svc := apikeys.New(db)
	owner := testutil.CreateUser(t, db)
	other := testutil.CreateUser(t, db)

	if _, _, err := svc.Create(ctx, owner.ID, "   "); code(t, err) != "invalid_name" {
		t.Errorf("empty name accepted: %v", err)
	}
	if _, _, err := svc.Create(ctx, owner.ID, strings.Repeat("x", 81)); code(t, err) != "invalid_name" {
		t.Errorf("long name accepted: %v", err)
	}

	key, plaintext, _ := svc.Create(ctx, owner.ID, "mine")
	for name, bad := range map[string]string{
		"empty":            "",
		"a session token":  "ses_tok_0000000000000000000000000000000000000000000000000000",
		"a binding secret": "bnd_sec_0000000000000000000000000000000000000000000000000000",
		"unknown key":      "mgt_tok_0000000000000000000000000000000000000000000000000000",
	} {
		if _, err := svc.Resolve(ctx, bad); code(t, err) != "invalid_credentials" {
			t.Errorf("%s accepted: %v", name, err)
		}
	}

	// One account's key is not another's to see or to end.
	if list, _ := svc.ListMine(ctx, other.ID); len(list) != 0 {
		t.Errorf("other sees %+v", list)
	}
	if err := svc.Revoke(ctx, other.ID, key.ID); code(t, err) != "key_not_found" {
		t.Errorf("other revoked it: %v", err)
	}
	if _, err := svc.Resolve(ctx, plaintext); err != nil {
		t.Errorf("key ended by someone else's revoke: %v", err)
	}
}
