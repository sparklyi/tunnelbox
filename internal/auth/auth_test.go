package auth

import (
	"context"
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
