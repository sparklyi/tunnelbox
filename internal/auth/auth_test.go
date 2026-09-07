package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

type memoryRepository struct {
	hash         []byte
	sessions     map[string]time.Time
	cleanupCalls int
}

func (r *memoryRepository) PasswordHash(context.Context) ([]byte, error) { return r.hash, nil }
func (r *memoryRepository) SavePasswordHash(_ context.Context, hash []byte) error {
	if len(r.hash) > 0 {
		return ErrAlreadySetup
	}
	r.hash = hash
	return nil
}
func (r *memoryRepository) ReplacePasswordHashAndSessions(_ context.Context, currentHash, newHash []byte, tokenHash string, expires time.Time) error {
	if len(r.hash) == 0 {
		return ErrNotInitialized
	}
	if string(r.hash) != string(currentHash) {
		return ErrInvalidCurrentPassword
	}
	r.hash = newHash
	clear(r.sessions)
	r.sessions[tokenHash] = expires
	return nil
}
func (r *memoryRepository) CreateSession(_ context.Context, token string, expires time.Time) error {
	r.sessions[token] = expires
	return nil
}
func (r *memoryRepository) SessionValid(_ context.Context, token string, now time.Time) (bool, error) {
	expires, ok := r.sessions[token]
	return ok && expires.After(now), nil
}
func (r *memoryRepository) DeleteSession(_ context.Context, token string) error {
	delete(r.sessions, token)
	return nil
}
func (r *memoryRepository) DeleteExpiredSessions(_ context.Context, now time.Time) error {
	r.cleanupCalls++
	for token, expires := range r.sessions {
		if !expires.After(now) {
			delete(r.sessions, token)
		}
	}
	return nil
}

func TestSetupLoginAndLogout(t *testing.T) {
	repository := &memoryRepository{sessions: make(map[string]time.Time)}
	manager := NewManager(repository)
	ctx := context.Background()

	token, err := manager.Setup(ctx, "correct horse battery")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	valid, err := manager.Authenticate(ctx, token)
	if err != nil || !valid {
		t.Fatalf("authenticate = %v, %v", valid, err)
	}
	if _, err := manager.Setup(ctx, "another password"); err != ErrAlreadySetup {
		t.Fatalf("second setup error = %v, want %v", err, ErrAlreadySetup)
	}
	repository.sessions["expired"] = time.Now().Add(-time.Minute)
	if _, err := manager.Login(ctx, "wrong password"); err != ErrUnauthenticated {
		t.Fatalf("wrong login error = %v, want %v", err, ErrUnauthenticated)
	}
	if repository.cleanupCalls != 3 {
		t.Fatalf("expired session cleanup calls = %d, want 3", repository.cleanupCalls)
	}
	if _, exists := repository.sessions["expired"]; exists {
		t.Fatal("expired session was not deleted")
	}
	if err := manager.Logout(ctx, token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	valid, err = manager.Authenticate(ctx, token)
	if err != nil || valid {
		t.Fatalf("authenticated after logout = %v, %v", valid, err)
	}
}

func TestSetupRejectsShortPassword(t *testing.T) {
	manager := NewManager(&memoryRepository{sessions: make(map[string]time.Time)})
	if _, err := manager.Setup(context.Background(), "short"); err != ErrInvalidPassword {
		t.Fatalf("setup error = %v, want %v", err, ErrInvalidPassword)
	}
}

func TestValidatePasswordBoundaries(t *testing.T) {
	for _, password := range []string{strings.Repeat("a", 72), strings.Repeat("密", 8)} {
		if err := validatePassword(password); err != nil {
			t.Fatalf("validate password of %d bytes: %v", len(password), err)
		}
	}
	for _, password := range []string{strings.Repeat("a", 7), strings.Repeat("a", 73), strings.Repeat("密", 25)} {
		if err := validatePassword(password); err != ErrInvalidPassword {
			t.Fatalf("validate password of %d bytes = %v, want %v", len(password), err, ErrInvalidPassword)
		}
	}
}

func TestChangePasswordRotatesSessions(t *testing.T) {
	repository := &memoryRepository{sessions: make(map[string]time.Time)}
	manager := NewManager(repository)
	ctx := context.Background()
	if _, err := manager.Setup(ctx, "old password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	oldSession, err := manager.Login(ctx, "old password")
	if err != nil {
		t.Fatalf("login old session: %v", err)
	}
	newSession, err := manager.ChangePassword(ctx, "old password", "new password")
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	if oldSession == newSession {
		t.Fatal("password change reused the old session")
	}
	valid, err := manager.Authenticate(ctx, oldSession)
	if err != nil || valid {
		t.Fatalf("old session valid = %v, err = %v", valid, err)
	}
	valid, err = manager.Authenticate(ctx, newSession)
	if err != nil || !valid {
		t.Fatalf("new session valid = %v, err = %v", valid, err)
	}
	if _, err := manager.Login(ctx, "old password"); err != ErrUnauthenticated {
		t.Fatalf("old password login error = %v, want %v", err, ErrUnauthenticated)
	}
	if _, err := manager.Login(ctx, "new password"); err != nil {
		t.Fatalf("new password login: %v", err)
	}
}

func TestChangePasswordRejectsInvalidCurrentPassword(t *testing.T) {
	repository := &memoryRepository{sessions: make(map[string]time.Time)}
	manager := NewManager(repository)
	token, err := manager.Setup(context.Background(), "old password")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := manager.ChangePassword(context.Background(), "wrong password", "new password"); err != ErrInvalidCurrentPassword {
		t.Fatalf("change password error = %v, want %v", err, ErrInvalidCurrentPassword)
	}
	valid, err := manager.Authenticate(context.Background(), token)
	if err != nil || !valid {
		t.Fatalf("original session valid = %v, err = %v", valid, err)
	}
}
