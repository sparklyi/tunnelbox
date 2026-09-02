package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("service not found")
	ErrConflict = errors.New("service already exists")
)

type State string

const (
	StateDraft     State = "draft"
	StateDeploying State = "deploying"
	StateActive    State = "active"
	StateError     State = "error"
)

type AllowType string

const (
	AllowEmail       AllowType = "email"
	AllowEmailDomain AllowType = "email_domain"
)

type RemoteRefs struct {
	TunnelID            string
	DNSRecordID         string
	AccessApplicationID string
	AccessPolicyID      string
}

type Service struct {
	ID          string
	WorkspaceID string
	Name        string
	Hostname    string
	OriginURL   string
	AllowType   AllowType
	AllowValue  string
	State       State
	RemoteRefs
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateInput struct {
	Name       string
	Hostname   string
	OriginURL  string
	AllowType  AllowType
	AllowValue string
}

type UpdateInput struct {
	Name       *string
	Hostname   *string
	OriginURL  *string
	AllowType  *AllowType
	AllowValue *string
}

// Repository is the storage contract consumed by the service use case.
type Repository interface {
	List(context.Context, string) ([]Service, error)
	Get(context.Context, string, string) (Service, error)
	Create(context.Context, Service) error
	Update(context.Context, Service) error
	Delete(context.Context, string, string) error
	SetState(context.Context, string, string, State) error
	SetRemoteRefs(context.Context, string, string, RemoteRefs) error
}

type UseCase struct {
	repo        Repository
	workspaceID string
	now         func() time.Time
}

func NewUseCase(repo Repository, workspaceID string) *UseCase {
	return &UseCase{repo: repo, workspaceID: workspaceID, now: time.Now}
}

func (u *UseCase) List(ctx context.Context) ([]Service, error) {
	return u.repo.List(ctx, u.workspaceID)
}

func (u *UseCase) Get(ctx context.Context, id string) (Service, error) {
	if strings.TrimSpace(id) == "" {
		return Service{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	return u.repo.Get(ctx, u.workspaceID, id)
}

func (u *UseCase) Create(ctx context.Context, input CreateInput) (Service, error) {
	name, hostname, origin, allowType, allowValue, err := normalizeAndValidate(input.Name, input.Hostname, input.OriginURL, input.AllowType, input.AllowValue)
	if err != nil {
		return Service{}, err
	}
	now := u.now().UTC()
	s := Service{
		ID:          newID("svc"),
		WorkspaceID: u.workspaceID,
		Name:        name,
		Hostname:    hostname,
		OriginURL:   origin,
		AllowType:   allowType,
		AllowValue:  allowValue,
		State:       StateDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := u.repo.Create(ctx, s); err != nil {
		return Service{}, err
	}
	return s, nil
}

func (u *UseCase) Update(ctx context.Context, id string, input UpdateInput) (Service, error) {
	current, err := u.Get(ctx, id)
	if err != nil {
		return Service{}, err
	}
	if current.State == StateDeploying {
		return Service{}, ErrConflict
	}
	name, hostname, origin, allowType, allowValue := current.Name, current.Hostname, current.OriginURL, current.AllowType, current.AllowValue
	if input.Name != nil {
		name = *input.Name
	}
	if input.Hostname != nil {
		hostname = *input.Hostname
	}
	if input.OriginURL != nil {
		origin = *input.OriginURL
	}
	if input.AllowType != nil {
		allowType = *input.AllowType
	}
	if input.AllowValue != nil {
		allowValue = *input.AllowValue
	}
	name, hostname, origin, allowType, allowValue, err = normalizeAndValidate(name, hostname, origin, allowType, allowValue)
	if err != nil {
		return Service{}, err
	}
	current.Name, current.Hostname, current.OriginURL = name, hostname, origin
	current.AllowType, current.AllowValue = allowType, allowValue
	current.UpdatedAt = u.now().UTC()
	if err := u.repo.Update(ctx, current); err != nil {
		return Service{}, err
	}
	return current, nil
}

func (u *UseCase) Delete(ctx context.Context, id string) error {
	current, err := u.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.State == StateDeploying {
		return ErrConflict
	}
	return u.repo.Delete(ctx, u.workspaceID, id)
}

func (u *UseCase) SetState(ctx context.Context, id string, state State) error {
	if state == "" {
		return &ValidationError{Field: "state", Message: "must not be empty"}
	}
	return u.repo.SetState(ctx, u.workspaceID, id, state)
}

func (u *UseCase) SetRemoteRefs(ctx context.Context, id string, refs RemoteRefs) error {
	return u.repo.SetRemoteRefs(ctx, u.workspaceID, id, refs)
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func normalizeAndValidate(name, hostname, origin string, allowType AllowType, allowValue string) (string, string, string, AllowType, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", "", "", &ValidationError{Field: "name", Message: "must not be empty"}
	}
	if len(name) > 120 {
		return "", "", "", "", "", &ValidationError{Field: "name", Message: "must be at most 120 characters"}
	}
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if !validHostname(hostname) {
		return "", "", "", "", "", &ValidationError{Field: "hostname", Message: "must be a valid DNS hostname"}
	}
	origin = strings.TrimSpace(origin)
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", "", "", "", &ValidationError{Field: "origin_url", Message: "must be an http or https URL without credentials"}
	}
	allowType = AllowType(strings.TrimSpace(string(allowType)))
	if allowType != AllowEmail && allowType != AllowEmailDomain {
		return "", "", "", "", "", &ValidationError{Field: "allow_type", Message: "must be email or email_domain"}
	}
	allowValue = strings.ToLower(strings.TrimSpace(allowValue))
	if allowType == AllowEmail {
		parsed, parseErr := mail.ParseAddress(allowValue)
		if parseErr != nil || parsed.Address != allowValue || !strings.Contains(allowValue, "@") {
			return "", "", "", "", "", &ValidationError{Field: "allow_value", Message: "must be a valid email address"}
		}
	} else {
		if !validHostname(allowValue) {
			return "", "", "", "", "", &ValidationError{Field: "allow_value", Message: "must be a valid email domain"}
		}
	}
	return name, hostname, origin, allowType, allowValue, nil
}

func validHostname(value string) bool {
	if value == "" || len(value) > 253 || strings.HasSuffix(value, ".") || net.ParseIP(value) != nil {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
