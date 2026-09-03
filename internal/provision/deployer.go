package provision

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
)

type Deployer struct {
	services   *service.UseCase
	operations *operation.Manager
	tunnel     TunnelPort
	access     AccessPort
	dns        DNSPort
	connector  ConnectorRuntime
	origin     OriginChecker
}

func NewDeployer(services *service.UseCase, operations *operation.Manager, tunnel TunnelPort, access AccessPort, dns DNSPort, connector ConnectorRuntime, origin OriginChecker) (*Deployer, error) {
	if services == nil || operations == nil || connector == nil {
		return nil, errors.New("deployer requires services, operations and connector")
	}
	return &Deployer{services: services, operations: operations, tunnel: tunnel, access: access, dns: dns, connector: connector, origin: origin}, nil
}

func (d *Deployer) Deploy(ctx context.Context, serviceID string) (operation.Operation, error) {
	item, err := d.services.Get(ctx, serviceID)
	if err != nil {
		return operation.Operation{}, err
	}
	if item.State == service.StateDeploying {
		return operation.Operation{}, service.ErrConflict
	}
	return d.operations.Start(ctx, serviceID, "deploy", func(taskCtx context.Context, op operation.Operation) error {
		return d.execute(taskCtx, op, item)
	})
}

// Stop starts an asynchronous shutdown of the service's local Connector.
// Cloudflare resources and the service configuration are intentionally kept so
// a later deployment can reuse them.
func (d *Deployer) Stop(ctx context.Context, serviceID string) (operation.Operation, error) {
	item, err := d.services.Get(ctx, serviceID)
	if err != nil {
		return operation.Operation{}, err
	}
	if item.State != service.StateActive && item.State != service.StateError && item.State != service.StateStopped {
		return operation.Operation{}, service.ErrConflict
	}
	return d.operations.Start(ctx, serviceID, "stop", func(taskCtx context.Context, op operation.Operation) error {
		return d.executeStop(taskCtx, op, item)
	})
}

// Delete removes a service and, when necessary, the Cloudflare resources
// created for it. Services that are already stopped and have no remote
// references are deleted immediately; cleanup that can call remote APIs runs
// through the operation manager so it can be retried after a restart.
func (d *Deployer) Delete(ctx context.Context, serviceID string) (operation.Operation, error) {
	item, err := d.services.Get(ctx, serviceID)
	if err != nil {
		return operation.Operation{}, err
	}
	if item.State == service.StateDeploying || item.State == service.StateStopping || item.State == service.StateActive {
		return operation.Operation{}, service.ErrConflict
	}
	if (item.State == service.StateDraft || item.State == service.StateStopped) && !hasCloudflareRefs(item.RemoteRefs) && item.PublicURL == "" {
		if err := d.services.Delete(ctx, serviceID); err != nil {
			return operation.Operation{}, err
		}
		return operation.Operation{}, nil
	}
	return d.operations.Start(ctx, serviceID, "delete", func(taskCtx context.Context, op operation.Operation) error {
		return d.executeDelete(taskCtx, op, item)
	})
}

// Resume returns a task for an operation loaded from storage after a process
// restart. The service is read again so remote references saved before the
// interruption are honored on the next attempt.
func (d *Deployer) Resume(op operation.Operation) operation.Task {
	if (op.Kind != "deploy" && op.Kind != "stop" && op.Kind != "delete") || op.ServiceID == "" {
		return nil
	}
	return func(ctx context.Context, current operation.Operation) error {
		item, err := d.services.Get(ctx, current.ServiceID)
		if err != nil {
			return adapterFailure(err, "service_unavailable", "service could not be loaded for operation")
		}
		if current.Kind == "stop" {
			return d.executeStop(ctx, current, item)
		}
		if current.Kind == "delete" {
			return d.executeDelete(ctx, current, item)
		}
		return d.execute(ctx, current, item)
	}
}

func (d *Deployer) executeDelete(ctx context.Context, op operation.Operation, item service.Service) error {
	refs := item.RemoteRefs
	setStep := func(step string) error {
		if err := d.operations.SetStep(ctx, op.ID, step); err != nil {
			return failure("operation_state_unavailable", "operation progress could not be saved", true)
		}
		return nil
	}
	setErrorState := func() {
		_ = d.services.SetState(context.Background(), item.ID, service.StateError)
	}
	fail := func(err error, code, message string) error {
		setErrorState()
		return adapterFailure(err, code, message)
	}
	persistRefs := func() error {
		if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
			setErrorState()
			return failure("service_state_unavailable", "remote resource state could not be saved", true)
		}
		return nil
	}

	if item.State != service.StateDraft {
		if err := d.services.SetState(ctx, item.ID, service.StateStopping); err != nil {
			return fail(err, "service_state_unavailable", "service state could not be updated")
		}
	}
	if err := setStep("connector_stop"); err != nil {
		return err
	}
	if err := d.connector.Stop(ctx, item.ID); err != nil {
		return fail(err, "connector_stop_failed", "cloudflared could not be stopped")
	}
	if item.State != service.StateDraft {
		if err := d.services.SetState(ctx, item.ID, service.StateStopped); err != nil {
			return fail(err, "service_state_unavailable", "service state could not be updated")
		}
	}
	if refs.PublicURL != "" {
		refs.PublicURL = ""
		if err := persistRefs(); err != nil {
			return err
		}
	}

	var tunnelDestroyer TunnelDestroyer
	if d.tunnel != nil {
		tunnelDestroyer, _ = d.tunnel.(TunnelDestroyer)
	}
	var accessDestroyer AccessDestroyer
	if d.access != nil {
		accessDestroyer, _ = d.access.(AccessDestroyer)
	}
	var dnsDestroyer DNSDestroyer
	if d.dns != nil {
		dnsDestroyer, _ = d.dns.(DNSDestroyer)
	}
	deleteRemote := func(step, id string, destroy func() error, clear func(*service.RemoteRefs), code, message string) error {
		if err := setStep(step); err != nil {
			return err
		}
		if id == "" {
			return nil
		}
		if destroy == nil {
			return fail(errors.New("cloudflare deletion adapter is not configured"), "cloudflare_not_configured", "Cloudflare integration is not configured")
		}
		if err := destroy(); err != nil {
			return fail(err, code, message)
		}
		clear(&refs)
		return persistRefs()
	}

	var deleteDNS func() error
	if dnsDestroyer != nil {
		deleteDNS = func() error { return dnsDestroyer.DeleteCNAME(ctx, refs.DNSRecordID) }
	}
	if err := deleteRemote("dns_delete", refs.DNSRecordID, deleteDNS, func(value *service.RemoteRefs) { value.DNSRecordID = "" }, "dns_delete_failed", "DNS CNAME could not be deleted"); err != nil {
		return err
	}

	var deletePolicy func() error
	if accessDestroyer != nil && refs.AccessApplicationID != "" {
		deletePolicy = func() error { return accessDestroyer.DeletePolicy(ctx, refs.AccessApplicationID, refs.AccessPolicyID) }
	}
	if err := deleteRemote("access_policy_delete", refs.AccessPolicyID, deletePolicy, func(value *service.RemoteRefs) { value.AccessPolicyID = "" }, "access_policy_delete_failed", "Access allow policy could not be deleted"); err != nil {
		return err
	}

	var deleteApplication func() error
	if accessDestroyer != nil {
		deleteApplication = func() error { return accessDestroyer.DeleteApplication(ctx, refs.AccessApplicationID) }
	}
	if err := deleteRemote("access_application_delete", refs.AccessApplicationID, deleteApplication, func(value *service.RemoteRefs) { value.AccessApplicationID = "" }, "access_application_delete_failed", "Access application could not be deleted"); err != nil {
		return err
	}

	var deletePrivateRoute func() error
	if tunnelDestroyer != nil {
		deletePrivateRoute = func() error { return tunnelDestroyer.DeletePrivateRoute(ctx, refs.PrivateRouteID) }
	}
	if err := deleteRemote("private_route_delete", refs.PrivateRouteID, deletePrivateRoute, func(value *service.RemoteRefs) { value.PrivateRouteID = "" }, "private_route_delete_failed", "private network route could not be deleted"); err != nil {
		return err
	}

	var deleteTunnel func() error
	if tunnelDestroyer != nil {
		deleteTunnel = func() error { return tunnelDestroyer.DeleteTunnel(ctx, refs.TunnelID) }
	}
	if err := deleteRemote("tunnel_delete", refs.TunnelID, deleteTunnel, func(value *service.RemoteRefs) { value.TunnelID = "" }, "tunnel_delete_failed", "Cloudflare Tunnel could not be deleted"); err != nil {
		return err
	}

	if err := setStep("service_delete"); err != nil {
		return err
	}
	if err := d.services.Delete(ctx, item.ID); err != nil {
		return fail(err, "service_delete_failed", "service record could not be deleted")
	}
	return nil
}

func hasCloudflareRefs(refs service.RemoteRefs) bool {
	return refs.TunnelID != "" || refs.PrivateRouteID != "" || refs.DNSRecordID != "" ||
		refs.AccessApplicationID != "" || refs.AccessPolicyID != ""
}

func (d *Deployer) executeStop(ctx context.Context, op operation.Operation, item service.Service) error {
	setStep := func(step string) error {
		if err := d.operations.SetStep(ctx, op.ID, step); err != nil {
			return failure("operation_state_unavailable", "operation progress could not be saved", true)
		}
		return nil
	}
	setErrorState := func() {
		_ = d.services.SetState(context.Background(), item.ID, service.StateError)
	}
	fail := func(err error, code, message string) error {
		setErrorState()
		return adapterFailure(err, code, message)
	}

	if err := d.services.SetState(ctx, item.ID, service.StateStopping); err != nil {
		return fail(err, "service_state_unavailable", "service state could not be updated")
	}
	if err := setStep("connector_stop"); err != nil {
		return err
	}
	if err := d.connector.Stop(ctx, item.ID); err != nil {
		return fail(err, "connector_stop_failed", "cloudflared could not be stopped")
	}
	if item.Mode == service.ModeQuick && item.PublicURL != "" {
		refs := item.RemoteRefs
		refs.PublicURL = ""
		if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
			return fail(err, "service_state_unavailable", "quick tunnel URL could not be cleared")
		}
	}
	if err := d.services.SetState(ctx, item.ID, service.StateStopped); err != nil {
		return fail(err, "service_state_unavailable", "service state could not be updated")
	}
	return nil
}

func (d *Deployer) execute(ctx context.Context, op operation.Operation, item service.Service) error {
	setStep := func(step string) error {
		if err := d.operations.SetStep(ctx, op.ID, step); err != nil {
			return failure("operation_state_unavailable", "operation progress could not be saved", true)
		}
		return nil
	}
	setErrorState := func() {
		_ = d.services.SetState(context.Background(), item.ID, service.StateError)
	}
	fail := func(err error, code, message string) error {
		setErrorState()
		return adapterFailure(err, code, message)
	}
	if err := d.services.SetState(ctx, item.ID, service.StateDeploying); err != nil {
		return fail(err, "service_state_unavailable", "service state could not be updated")
	}
	if err := setStep("origin_check"); err != nil {
		return err
	}
	if d.origin != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := d.origin.Check(checkCtx, item.OriginURL)
		cancel()
		if err != nil {
			return fail(err, "origin_unreachable", "origin cannot be reached from connector")
		}
	}
	if item.Mode == "" {
		item.Mode = service.ModePublic
	}

	// Quick tunnels are deliberately independent from the Cloudflare API. They
	// provide a temporary share URL for development and do not create Access or
	// DNS resources.
	if item.Mode == service.ModeQuick {
		if err := setStep("quick_tunnel"); err != nil {
			return err
		}
		if err := d.connector.EnsureRunning(ctx, ConnectorSpec{ServiceID: item.ID, OriginURL: item.OriginURL, Quick: true}); err != nil {
			return fail(err, "quick_tunnel_start_failed", "quick tunnel could not be started")
		}
		if err := setStep("connector_health"); err != nil {
			return err
		}
		if err := d.waitConnector(ctx, item.ID, true); err != nil {
			return fail(err, "connector_unhealthy", "quick tunnel did not become ready")
		}
		status, err := d.connector.Status(ctx, item.ID)
		if err != nil {
			return fail(err, "quick_tunnel_url_unavailable", "quick tunnel did not provide a public URL")
		}
		if !status.Running || !status.Healthy || strings.TrimSpace(status.URL) == "" {
			return fail(errors.New("quick tunnel is no longer running"), "connector_unhealthy", "quick tunnel did not remain ready")
		}
		refs := item.RemoteRefs
		refs.PublicURL = status.URL
		if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
			return fail(err, "service_state_unavailable", "quick tunnel URL could not be saved")
		}
		if err := d.services.SetState(ctx, item.ID, service.StateActive); err != nil {
			return fail(err, "service_state_unavailable", "service state could not be updated")
		}
		return nil
	}
	if item.Mode != service.ModePrivate && item.Mode != service.ModePublic {
		return fail(errors.New("unsupported service mode"), "invalid_mode", "service mode is not supported")
	}
	if d.tunnel == nil || d.access == nil || (item.Mode == service.ModePublic && d.dns == nil) {
		return fail(errors.New("cloudflare adapters are not configured"), "cloudflare_not_configured", "Cloudflare integration is not configured")
	}

	refs := item.RemoteRefs
	if err := setStep("tunnel"); err != nil {
		return err
	}
	tunnel, err := d.tunnel.EnsureTunnel(ctx, TunnelSpec{ID: refs.TunnelID, Name: "tunnelbox-" + item.ID})
	if err != nil {
		return fail(err, "tunnel_unavailable", "tunnel could not be created or updated")
	}
	refs.TunnelID = tunnel.ID
	if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
		return fail(err, "service_state_unavailable", "tunnel reference could not be saved")
	}

	if err := setStep("tunnel_route"); err != nil {
		return err
	}
	if item.Mode == service.ModePrivate {
		network, err := privateNetwork(item)
		if err != nil {
			return fail(err, "private_target_invalid", "private service target is invalid")
		}
		if err := d.tunnel.ApplyWebRoute(ctx, RouteSpec{TunnelID: refs.TunnelID, Private: true}); err != nil {
			return fail(err, "tunnel_route_failed", "private tunnel routing could not be enabled")
		}
		route, err := d.tunnel.EnsurePrivateRoute(ctx, PrivateRouteSpec{ID: refs.PrivateRouteID, Network: network, TunnelID: refs.TunnelID, Comment: item.Name})
		if err != nil {
			return fail(err, "private_route_failed", "private network route could not be created or updated")
		}
		refs.PrivateRouteID = route.ID
		if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
			return fail(err, "service_state_unavailable", "private route reference could not be saved")
		}
	} else {
		if err := d.tunnel.ApplyWebRoute(ctx, RouteSpec{TunnelID: refs.TunnelID, Hostname: item.Hostname, OriginURL: item.OriginURL}); err != nil {
			return fail(err, "tunnel_route_failed", "tunnel route could not be applied")
		}
	}

	if err := setStep("connector"); err != nil {
		return err
	}
	if err := d.connector.EnsureRunning(ctx, ConnectorSpec{ServiceID: item.ID, TunnelID: refs.TunnelID, Token: tunnel.ConnectorToken}); err != nil {
		return fail(err, "connector_start_failed", "cloudflared could not be started")
	}
	if err := setStep("connector_health"); err != nil {
		return err
	}
	if err := d.waitConnector(ctx, item.ID, false); err != nil {
		return fail(err, "connector_unhealthy", "cloudflared did not become healthy")
	}

	if err := setStep("access_application"); err != nil {
		return err
	}
	accessDomain := item.Hostname
	if item.Mode == service.ModePrivate {
		accessDomain, err = privateAccessDomain(item)
		if err != nil {
			return fail(err, "private_target_invalid", "private service target is invalid")
		}
	}
	application, err := d.access.EnsureApplication(ctx, AccessApplicationSpec{ID: refs.AccessApplicationID, Name: item.Name, Domain: accessDomain, Private: item.Mode == service.ModePrivate})
	if err != nil {
		return fail(err, "access_application_failed", "Access application could not be created or updated")
	}
	refs.AccessApplicationID = application.ID
	if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
		return fail(err, "service_state_unavailable", "Access application reference could not be saved")
	}

	if err := setStep("access_policy"); err != nil {
		return err
	}
	policy, err := d.access.EnsurePolicy(ctx, AccessPolicySpec{ID: refs.AccessPolicyID, ApplicationID: refs.AccessApplicationID,
		Name: item.Name + " Allow", AllowType: string(item.AllowType), AllowValue: item.AllowValue})
	if err != nil {
		return fail(err, "access_policy_failed", "Access allow policy could not be created or updated")
	}
	refs.AccessPolicyID = policy.ID
	if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
		return fail(err, "service_state_unavailable", "Access policy reference could not be saved")
	}

	// Access policies are created below an application, but the application
	// still needs an explicit policy attachment before it is reachable.
	if err := setStep("access_policy_attach"); err != nil {
		return err
	}
	application, err = d.access.EnsureApplication(ctx, AccessApplicationSpec{
		ID: refs.AccessApplicationID, Name: item.Name, Domain: accessDomain, PolicyID: refs.AccessPolicyID, Private: item.Mode == service.ModePrivate,
	})
	if err != nil {
		return fail(err, "access_policy_attach_failed", "Access allow policy could not be attached")
	}
	if application.ID != "" {
		refs.AccessApplicationID = application.ID
		if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
			return fail(err, "service_state_unavailable", "Access application reference could not be saved")
		}
	}

	if item.Mode != service.ModePrivate {
		if err := setStep("dns"); err != nil {
			return err
		}
		dnsRecord, err := d.dns.EnsureCNAME(ctx, CNAMESpec{ID: refs.DNSRecordID, Name: item.Hostname, Target: refs.TunnelID + ".cfargotunnel.com", Proxied: true})
		if err != nil {
			return fail(err, "dns_failed", "DNS CNAME could not be created or updated")
		}
		refs.DNSRecordID = dnsRecord.ID
	}
	if err := d.services.SetRemoteRefs(ctx, item.ID, refs); err != nil {
		return fail(err, "service_state_unavailable", "remote references could not be saved")
	}
	if err := d.services.SetState(ctx, item.ID, service.StateActive); err != nil {
		return fail(err, "service_state_unavailable", "service state could not be updated")
	}
	return nil
}

func (d *Deployer) waitConnector(ctx context.Context, serviceID string, quick bool) error {
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := d.connector.Status(checkCtx, serviceID)
		if err != nil {
			return err
		}
		if status.Message == "process exited" {
			return errors.New("connector process exited before becoming healthy")
		}
		if status.Running && status.Healthy && (!quick || strings.TrimSpace(status.URL) != "") {
			return nil
		}
		select {
		case <-checkCtx.Done():
			return checkCtx.Err()
		case <-ticker.C:
		}
	}
}

func privateNetwork(item service.Service) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(item.Hostname))
	if ip == nil {
		return "", errors.New("private target must be an IP address")
	}
	if ip.To4() != nil {
		return ip.String() + "/32", nil
	}
	return ip.String() + "/128", nil
}

func privateAccessDomain(item service.Service) (string, error) {
	u, err := url.Parse(item.OriginURL)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(item.Hostname, port), nil
}

func adapterFailure(err error, fallbackCode, message string) error {
	if err == nil {
		return failure(fallbackCode, message, false)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var coded CodedError
	if errors.As(err, &coded) {
		code := coded.FailureCode()
		if code == "" {
			code = fallbackCode
		}
		return failure(code, message, coded.RemoteStateUnknown())
	}
	return failure(fallbackCode, message, false)
}

func failure(code, message string, unknown bool) error {
	return &operation.Failure{Code: strings.TrimSpace(code), Message: strings.TrimSpace(message), Unknown: unknown}
}

var _ interface {
	Deploy(context.Context, string) (operation.Operation, error)
	Stop(context.Context, string) (operation.Operation, error)
	Delete(context.Context, string) (operation.Operation, error)
} = (*Deployer)(nil)
