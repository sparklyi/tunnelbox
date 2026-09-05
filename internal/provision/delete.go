package provision

import (
	"context"
	"errors"

	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
)

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
	if err := setStep("connector_credentials_delete"); err != nil {
		return err
	}
	if err := d.connector.DeleteCredentials(ctx, item.ID); err != nil {
		return fail(err, "connector_token_delete_failed", "connector credentials could not be deleted")
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
