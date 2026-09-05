package provision

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/sparklyi/tunnelbox/internal/service"
)

func (d *Deployer) executeManaged(ctx context.Context, item service.Service, setStep func(string) error, fail func(error, string, string) error) error {
	if d.tunnel == nil || d.access == nil || (item.Mode == service.ModePublic && d.dns == nil) {
		return fail(errors.New("cloudflare adapters are not configured"), "cloudflare_not_configured", "Cloudflare integration is not configured")
	}
	if item.Mode == service.ModePublic {
		if err := setStep("zone_validation"); err != nil {
			return err
		}
		if err := d.dns.ValidateHostname(ctx, item.Hostname); err != nil {
			return fail(err, "hostname_not_in_zone", "hostname does not belong to the selected Cloudflare zone")
		}
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
