package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const SessionCookie = "tunnelbox_session"

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrAlreadySetup    = errors.New("administrator is already configured")
	ErrUnauthenticated = errors.New("authentication required")
)

type Repository interface {
	PasswordHash(context.Context) ([]byte, error)
	SavePasswordHash(context.Context, []byte) error
	CreateSession(context.Context, string, time.Time) error
	SessionValid(context.Context, string, time.Time) (bool, error)
	DeleteSession(context.Context, string) error
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
	hash, err := m.repository.PasswordHash(ctx)
	if err != nil {
		return "", err
	}
	if len(hash) == 0 || bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return "", ErrUnauthenticated
	}
	return m.createSession(ctx)
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
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw[:])
	if err := m.repository.CreateSession(ctx, hashToken(token), m.now().Add(30*24*time.Hour)); err != nil {
		return "", err
	}
	return token, nil
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 256 {
		return ErrInvalidPassword
	}
	return nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
