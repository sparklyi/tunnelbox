package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/sparklyi/tunnelbox/internal/auth"
	"github.com/sparklyi/tunnelbox/internal/provision"
	"github.com/sparklyi/tunnelbox/internal/service"
)

// These aliases keep the adapter API convenient while the actual application
// contract remains owned by provision.
type ConfigureInput = provision.CloudflareConfigureInput
type IntegrationStatus = provision.CloudflareIntegrationStatus

type SettingsStore interface {
	GetWorkspace(context.Context, string) (service.Workspace, error)
	SaveCloudflareConfig(context.Context, string, string, string, string) error
}

type Integration struct {
	store       SettingsStore
	workspaceID string
	tokenPath   string
	baseURL     string
	httpClient  *http.Client

	mu     sync.RWMutex
	client *Client
	status IntegrationStatus
}

func NewIntegration(ctx context.Context, store SettingsStore, workspaceID, defaultTokenPath, baseURL string, httpClient *http.Client) (*Integration, error) {
	if store == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, errors.New("cloudflare integration requires a workspace store and id")
	}
	workspace, err := store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load workspace settings: %w", err)
	}
	tokenPath := strings.TrimSpace(workspace.CloudflareTokenPath)
	if tokenPath == "" {
		tokenPath = strings.TrimSpace(defaultTokenPath)
	}
	integration := &Integration{store: store, workspaceID: workspaceID, tokenPath: tokenPath, baseURL: baseURL, httpClient: httpClient}
	if workspace.AccountID == "" || workspace.ZoneID == "" || tokenPath == "" {
		return integration, nil
	}
	token, loadErr := auth.LoadToken(tokenPath)
	if loadErr != nil {
		integration.status = IntegrationStatus{AccountID: workspace.AccountID, ZoneID: workspace.ZoneID, LastError: "cloudflare token file is unavailable"}
		return integration, nil
	}
	client, clientErr := New(Config{Token: token, AccountID: workspace.AccountID, ZoneID: workspace.ZoneID, BaseURL: baseURL, HTTPClient: httpClient})
	if clientErr != nil {
		return nil, clientErr
	}
	integration.client = client
	integration.status = IntegrationStatus{Configured: true, AccountID: workspace.AccountID, ZoneID: workspace.ZoneID}
	return integration, nil
}

func (i *Integration) Configure(ctx context.Context, input ConfigureInput) (IntegrationStatus, error) {
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.ZoneID = strings.TrimSpace(input.ZoneID)
	input.Token = strings.TrimSpace(input.Token)
	if input.AccountID == "" || input.ZoneID == "" || input.Token == "" {
		return IntegrationStatus{}, &Error{Code: "cloudflare_configuration_invalid"}
	}
	client, err := New(Config{Token: input.Token, AccountID: input.AccountID, ZoneID: input.ZoneID, BaseURL: i.baseURL, HTTPClient: i.httpClient})
	if err != nil {
		return IntegrationStatus{}, &Error{Code: "cloudflare_configuration_invalid", Cause: err}
	}
	tokenStatus, err := client.VerifyToken(ctx)
	if err != nil {
		return IntegrationStatus{}, err
	}
	if tokenStatus.Status != "active" {
		return IntegrationStatus{}, &Error{Code: "cloudflare_token_inactive"}
	}
	zones, err := client.Zones(ctx)
	if err != nil {
		return IntegrationStatus{}, err
	}
	found := false
	for _, zone := range zones {
		if zone.ID == input.ZoneID {
			found = true
			break
		}
	}
	if !found {
		return IntegrationStatus{}, &Error{Code: "cloudflare_zone_not_available"}
	}
	if i.tokenPath == "" {
		return IntegrationStatus{}, &Error{Code: "cloudflare_token_path_unconfigured"}
	}
	if err := auth.SaveToken(i.tokenPath, input.Token); err != nil {
		return IntegrationStatus{}, &Error{Code: "cloudflare_token_file_failed", Cause: err}
	}
	if err := i.store.SaveCloudflareConfig(ctx, i.workspaceID, input.AccountID, input.ZoneID, i.tokenPath); err != nil {
		return IntegrationStatus{}, err
	}
	status := IntegrationStatus{Configured: true, AccountID: input.AccountID, ZoneID: input.ZoneID, TokenID: tokenStatus.ID, TokenState: tokenStatus.Status}
	i.mu.Lock()
	i.client = client
	i.status = status
	i.mu.Unlock()
	return status, nil
}

func (i *Integration) Status(ctx context.Context) (IntegrationStatus, error) {
	i.mu.RLock()
	client := i.client
	status := i.status
	i.mu.RUnlock()
	if client == nil {
		return status, nil
	}
	tokenStatus, err := client.VerifyToken(ctx)
	if err != nil {
		status.LastError = "cloudflare token verification failed"
		i.mu.Lock()
		i.status = status
		i.mu.Unlock()
		return status, nil
	}
	status.Configured = true
	status.TokenID = tokenStatus.ID
	status.TokenState = tokenStatus.Status
	status.LastError = ""
	i.mu.Lock()
	i.status = status
	i.mu.Unlock()
	return status, nil
}

func (i *Integration) Zones(ctx context.Context) ([]Zone, error) {
	client, err := i.current()
	if err != nil {
		return nil, err
	}
	return client.Zones(ctx)
}

func (i *Integration) current() (*Client, error) {
	i.mu.RLock()
	client := i.client
	i.mu.RUnlock()
	if client == nil {
		return nil, &Error{Code: "cloudflare_not_configured"}
	}
	return client, nil
}

func (i *Integration) EnsureTunnel(ctx context.Context, spec provision.TunnelSpec) (provision.RemoteTunnel, error) {
	client, err := i.current()
	if err != nil {
		return provision.RemoteTunnel{}, err
	}
	return client.EnsureTunnel(ctx, spec)
}

func (i *Integration) ApplyWebRoute(ctx context.Context, spec provision.RouteSpec) error {
	client, err := i.current()
	if err != nil {
		return err
	}
	return client.ApplyWebRoute(ctx, spec)
}

func (i *Integration) EnsureApplication(ctx context.Context, spec provision.AccessApplicationSpec) (provision.RemoteRef, error) {
	client, err := i.current()
	if err != nil {
		return provision.RemoteRef{}, err
	}
	return client.EnsureApplication(ctx, spec)
}

func (i *Integration) EnsurePolicy(ctx context.Context, spec provision.AccessPolicySpec) (provision.RemoteRef, error) {
	client, err := i.current()
	if err != nil {
		return provision.RemoteRef{}, err
	}
	return client.EnsurePolicy(ctx, spec)
}

func (i *Integration) EnsureCNAME(ctx context.Context, spec provision.CNAMESpec) (provision.RemoteRef, error) {
	client, err := i.current()
	if err != nil {
		return provision.RemoteRef{}, err
	}
	return client.EnsureCNAME(ctx, spec)
}

var _ provision.TunnelPort = (*Integration)(nil)
var _ provision.AccessPort = (*Integration)(nil)
var _ provision.DNSPort = (*Integration)(nil)
