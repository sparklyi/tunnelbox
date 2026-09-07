package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	SessionCookie         = "tunnelbox_session"
	minPasswordCharacters = 8
	maxPasswordBytes      = 72
)

var (
	ErrInvalidPassword        = errors.New("invalid password")
	ErrInvalidCurrentPassword = errors.New("current password is invalid")
	ErrAlreadySetup           = errors.New("administrator is already configured")
	ErrNotInitialized         = errors.New("administrator is not configured")
	ErrUnauthenticated        = errors.New("authentication required")
)

type Repository interface {
	PasswordHash(context.Context) ([]byte, error)
	SavePasswordHash(context.Context, []byte) error
	ReplacePasswordHashAndSessions(context.Context, []byte, []byte, string, time.Time) error
	CreateSession(context.Context, string, time.Time) error
	SessionValid(context.Context, string, time.Time) (bool, error)
	DeleteSession(context.Context, string) error
	DeleteExpiredSessions(context.Context, time.Time) error
}

type Manager struct {
	repository Repository
	now        func() time.Time
}

func NewManager(repository Repository) *Manager {
	return &Manager{repository: repository, now: time.Now}
}

func (m *Manager) Initialized(ctx context.Context) (bool, error) {
	hash, err := m.repository.PasswordHash(ctx)
	return len(hash) > 0, err
}

func (m *Manager) Setup(ctx context.Context, password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	if err := m.repository.DeleteExpiredSessions(ctx, m.now()); err != nil {
		return "", err
	}
	hash, err := m.repository.PasswordHash(ctx)
	if err != nil {
		return "", err
	}
	if len(hash) > 0 {
		return "", ErrAlreadySetup
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := m.repository.SavePasswordHash(ctx, passwordHash); err != nil {
		return "", err
	}
	return m.createSession(ctx)
}

func (m *Manager) Login(ctx context.Context, password string) (string, error) {
	if err := m.repository.DeleteExpiredSessions(ctx, m.now()); err != nil {
		return "", err
	}
	hash, err := m.repository.PasswordHash(ctx)
	if err != nil {
		return "", err
	}
	if len(hash) == 0 || bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return "", ErrUnauthenticated
	}
	return m.createSession(ctx)
}

// ChangePassword verifies the current password, rotates the stored hash, and
// replaces every existing session with a fresh session for the caller.
func (m *Manager) ChangePassword(ctx context.Context, currentPassword, newPassword string) (string, error) {
	if err := validatePassword(newPassword); err != nil {
		return "", err
	}
	hash, err := m.repository.PasswordHash(ctx)
	if err != nil {
		return "", err
	}
	if len(hash) == 0 {
		return "", ErrNotInitialized
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(currentPassword)) != nil {
		return "", ErrInvalidCurrentPassword
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", err
	}
	if err := m.repository.ReplacePasswordHashAndSessions(ctx, hash, passwordHash, tokenHash, m.now().Add(30*24*time.Hour)); err != nil {
		return "", err
	}
	return token, nil
}

func (m *Manager) Authenticate(ctx context.Context, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	return m.repository.SessionValid(ctx, hashToken(token), m.now())
}

func (m *Manager) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return m.repository.DeleteSession(ctx, hashToken(token))
}

func (m *Manager) createSession(ctx context.Context) (string, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", err
	}
	if err := m.repository.CreateSession(ctx, tokenHash, m.now().Add(30*24*time.Hour)); err != nil {
		return "", err
	}
	return token, nil
}

func newSessionToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw[:])
	return token, hashToken(token), nil
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < minPasswordCharacters || len(password) > maxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
