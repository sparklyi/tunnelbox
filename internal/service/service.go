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
	StateStopping  State = "stopping"
	StateActive    State = "active"
	StateStopped   State = "stopped"
	StateError     State = "error"
)

// Mode controls how a service is exposed.
type Mode string

const (
	ModeQuick   Mode = "quick"
	ModePrivate Mode = "private"
	ModePublic  Mode = "public"
)

type AllowType string

const (
	AllowEmail       AllowType = "email"
	AllowEmailDomain AllowType = "email_domain"
)

type RemoteRefs struct {
	TunnelID            string
	PrivateRouteID      string
	DNSRecordID         string
	AccessApplicationID string
	AccessPolicyID      string
	PublicURL           string
}

type Service struct {
	ID          string
	WorkspaceID string
	Name        string
	Mode        Mode
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
	Mode       Mode
	Hostname   string
	OriginURL  string
	AllowType  AllowType
	AllowValue string
}

type UpdateInput struct {
	Name       *string
	Mode       *Mode
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

// ReconcileQuickServices clears completed Quick URLs after a control-plane
// restart. Quick Tunnel URLs belong to an in-memory cloudflared process and
// cannot be restored from SQLite, so the service must be deployed again.
func (u *UseCase) ReconcileQuickServices(ctx context.Context) error {
	items, err := u.List(ctx)
	if err != nil {
		return fmt.Errorf("list services for quick reconciliation: %w", err)
	}
	for _, item := range items {
		if item.Mode != ModeQuick || (item.State != StateActive && item.PublicURL == "") {
			continue
		}
		refs := item.RemoteRefs
		refs.PublicURL = ""
		if err := u.SetRemoteRefs(ctx, item.ID, refs); err != nil {
			return fmt.Errorf("clear quick URL for %s: %w", item.ID, err)
		}
		if item.State == StateActive {
			if err := u.SetState(ctx, item.ID, StateDraft); err != nil {
				return fmt.Errorf("reset quick service %s: %w", item.ID, err)
			}
		}
	}
	return nil
}

func (u *UseCase) Get(ctx context.Context, id string) (Service, error) {
	if strings.TrimSpace(id) == "" {
		return Service{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	return u.repo.Get(ctx, u.workspaceID, id)
}

func (u *UseCase) Create(ctx context.Context, input CreateInput) (Service, error) {
	mode := normalizeMode(input.Mode)
	// Requests from the original API omitted mode but supplied a public
	// hostname and Access condition. Keep those clients on the public path;
	// an otherwise empty new request defaults to quick mode.
	if strings.TrimSpace(string(input.Mode)) == "" &&
		(strings.TrimSpace(input.Hostname) != "" || input.AllowType != "" || strings.TrimSpace(input.AllowValue) != "") {
		mode = ModePublic
	}
	name, hostname, origin, allowType, allowValue, err := normalizeAndValidate(mode, input.Name, input.Hostname, input.OriginURL, input.AllowType, input.AllowValue)
	if err != nil {
		return Service{}, err
	}
	id := newID("svc")
	if mode == ModeQuick && hostname == "" {
		// Keep a non-empty compatibility key for the legacy unique hostname
		// constraint. This value is never exposed as a public address.
		hostname = "quick-" + id + ".invalid"
	}
	now := u.now().UTC()
	s := Service{
		ID:          id,
		WorkspaceID: u.workspaceID,
		Name:        name,
		Mode:        mode,
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
	if current.State == StateDeploying || current.State == StateStopping || current.State == StateActive {
		return Service{}, ErrConflict
	}
	mode := current.Mode
	if mode == "" {
		mode = ModePublic
	}
	name, hostname, origin, allowType, allowValue := current.Name, current.Hostname, current.OriginURL, current.AllowType, current.AllowValue
	if input.Name != nil {
		name = *input.Name
	}
	if input.Mode != nil {
		mode = *input.Mode
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
	currentMode := current.Mode
	if currentMode == "" {
		currentMode = ModePublic
	}
	mode = normalizeMode(mode)
	if mode != currentMode && hasRemoteRefs(current.RemoteRefs) {
		return Service{}, ErrConflict
	}
	// Quick services keep a legacy non-empty hostname in SQLite solely for the
	// old NOT NULL/unique constraint. Treat that sentinel as an empty input when
	// validating or switching modes.
	if isQuickPlaceholder(hostname) {
		hostname = ""
	}
	name, hostname, origin, allowType, allowValue, err = normalizeAndValidate(mode, name, hostname, origin, allowType, allowValue)
	if err != nil {
		return Service{}, err
	}
	if mode == ModeQuick && hostname == "" {
		hostname = current.Hostname
		if hostname == "" {
			hostname = "quick-" + current.ID + ".invalid"
		}
	}
	current.Name, current.Mode, current.Hostname, current.OriginURL = name, mode, hostname, origin
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
	// A service with a remote reference needs an explicit undeploy workflow
	// before deletion; removing only the local row would orphan Cloudflare
	// resources and a managed connector process.
	if current.State == StateDeploying || current.State == StateStopping || current.State == StateActive || hasRemoteRefs(current.RemoteRefs) {
		return ErrConflict
	}
	return u.repo.Delete(ctx, u.workspaceID, id)
}

func hasRemoteRefs(refs RemoteRefs) bool {
	return refs.TunnelID != "" || refs.PrivateRouteID != "" || refs.DNSRecordID != "" ||
		refs.AccessApplicationID != "" || refs.AccessPolicyID != "" || refs.PublicURL != ""
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

func normalizeMode(mode Mode) Mode {
	mode = Mode(strings.ToLower(strings.TrimSpace(string(mode))))
	if mode == "" {
		return ModeQuick
	}
	return mode
}

func normalizeAndValidate(mode Mode, name, hostname, origin string, allowType AllowType, allowValue string) (string, string, string, AllowType, string, error) {
	if mode != ModeQuick && mode != ModePrivate && mode != ModePublic {
		return "", "", "", "", "", &ValidationError{Field: "mode", Message: "must be quick, private or public"}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", "", "", &ValidationError{Field: "name", Message: "must not be empty"}
	}
	if len(name) > 120 {
		return "", "", "", "", "", &ValidationError{Field: "name", Message: "must be at most 120 characters"}
	}
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if isQuickPlaceholder(hostname) {
		hostname = ""
	}
	switch mode {
	case ModeQuick:
		if hostname != "" {
			return "", "", "", "", "", &ValidationError{Field: "hostname", Message: "quick mode does not use a hostname"}
		}
	case ModePrivate:
		if !validPrivateIP(hostname) {
			return "", "", "", "", "", &ValidationError{Field: "hostname", Message: "private mode requires a private IP address"}
		}
	case ModePublic:
		if !validHostname(hostname) {
			return "", "", "", "", "", &ValidationError{Field: "hostname", Message: "must be a valid DNS hostname"}
		}
	}
	origin = strings.TrimSpace(origin)
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", "", "", "", &ValidationError{Field: "origin_url", Message: "must be an http or https URL without credentials"}
	}
	if mode == ModePrivate {
		originIP := net.ParseIP(u.Hostname())
		if originIP == nil || !originIP.Equal(net.ParseIP(hostname)) {
			return "", "", "", "", "", &ValidationError{Field: "origin_url", Message: "private mode origin host must match the private IP target"}
		}
	}
	allowType = AllowType(strings.TrimSpace(string(allowType)))
	allowValue = strings.ToLower(strings.TrimSpace(allowValue))
	if mode == ModeQuick {
		if allowType == "" && allowValue == "" {
			return name, hostname, origin, "", "", nil
		}
		return "", "", "", "", "", &ValidationError{Field: "allow_type", Message: "quick mode does not use an Access policy"}
	}
	if allowType != AllowEmail && allowType != AllowEmailDomain {
		return "", "", "", "", "", &ValidationError{Field: "allow_type", Message: "must be email or email_domain"}
	}
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

func validPrivateIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || !ip.IsPrivate() {
		return false
	}
	return !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLinkLocalUnicast()
}

func isQuickPlaceholder(value string) bool {
	return strings.HasPrefix(value, "quick-svc_") && strings.HasSuffix(value, ".invalid")
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
