package provision

import (
	"context"
	"errors"
	"strings"

	"github.com/sparklyi/tunnelbox/internal/service"
)

// Quick tunnels are independent from the Cloudflare API. They provide a
// temporary share URL and do not create Access or DNS resources.
func (d *Deployer) executeQuick(ctx context.Context, item service.Service, setStep func(string) error, fail func(error, string, string) error) error {
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
