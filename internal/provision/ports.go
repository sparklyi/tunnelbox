package provision

import "context"

// CodedError lets adapters provide a safe, stable failure code without
// exposing SDK or process error text to the operation API.
type CodedError interface {
	error
	FailureCode() string
	RemoteStateUnknown() bool
}

type TunnelSpec struct {
	ID   string
	Name string
}

type RemoteTunnel struct {
	ID             string
	Name           string
	ConnectorToken string
}

type RouteSpec struct {
	TunnelID  string
	Hostname  string
	OriginURL string
	Private   bool
}

type PrivateRouteSpec struct {
	ID       string
	Network  string
	TunnelID string
	Comment  string
}

type AccessApplicationSpec struct {
	ID       string
	Name     string
	Domain   string
	PolicyID string
	Private  bool
}

type AccessPolicySpec struct {
	ID            string
	ApplicationID string
	Name          string
	AllowType     string
	AllowValue    string
}

type RemoteRef struct {
	ID string
}

type CNAMESpec struct {
	ID      string
	Name    string
	Target  string
	Proxied bool
}

type ConnectorSpec struct {
	ServiceID string
	TunnelID  string
	Token     string
	OriginURL string
	Quick     bool
}

type ConnectorStatus struct {
	ServiceID string
	Mode      string
	Running   bool
	Healthy   bool
	URL       string
	Message   string
}

// CloudflareConfigureInput contains the account-scoped settings needed by the
// control plane. The token is accepted only at the configuration boundary and
// is never part of a status response or persisted domain object.
type CloudflareConfigureInput struct {
	AccountID string
	ZoneID    string
	Token     string
}

type CloudflareIntegrationStatus struct {
	Configured bool
	AccountID  string
	ZoneID     string
	TokenID    string
	TokenState string
	LastError  string
}

type Zone struct {
	ID   string
	Name string
}

// CloudflareIntegration is the application contract used by the HTTP layer
// and deployment use case; SDK types stay inside the cloudflare adapter.
type CloudflareIntegration interface {
	Configure(context.Context, CloudflareConfigureInput) (CloudflareIntegrationStatus, error)
	Status(context.Context) (CloudflareIntegrationStatus, error)
	Zones(context.Context) ([]Zone, error)
}

// ConnectorLister is optional at the transport boundary and lets the control
// plane expose the currently managed cloudflared processes.
type ConnectorLister interface {
	List(context.Context) ([]ConnectorStatus, error)
}

type TunnelPort interface {
	EnsureTunnel(context.Context, TunnelSpec) (RemoteTunnel, error)
	ApplyWebRoute(context.Context, RouteSpec) error
	EnsurePrivateRoute(context.Context, PrivateRouteSpec) (RemoteRef, error)
}

type AccessPort interface {
	EnsureApplication(context.Context, AccessApplicationSpec) (RemoteRef, error)
	EnsurePolicy(context.Context, AccessPolicySpec) (RemoteRef, error)
}

type DNSPort interface {
	EnsureCNAME(context.Context, CNAMESpec) (RemoteRef, error)
}

// ConnectorRuntime manages one cloudflared process per service identifier.
type ConnectorRuntime interface {
	EnsureRunning(context.Context, ConnectorSpec) error
	Reload(context.Context, string) error
	Status(context.Context, string) (ConnectorStatus, error)
	Stop(context.Context, string) error
}

type OriginChecker interface {
	Check(context.Context, string) error
}
