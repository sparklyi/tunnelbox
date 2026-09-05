package provision

import (
	"context"
	"errors"
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
	if item.State == service.StateDraft && !hasCloudflareRefs(item.RemoteRefs) && item.PublicURL == "" {
		if err := d.connector.DeleteCredentials(ctx, item.ID); err != nil {
			return operation.Operation{}, adapterFailure(err, "connector_token_delete_failed", "connector credentials could not be deleted")
		}
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
	switch item.Mode {
	case service.ModeQuick:
		return d.executeQuick(ctx, item, setStep, fail)
	case service.ModePrivate, service.ModePublic:
		return d.executeManaged(ctx, item, setStep, fail)
	default:
		return fail(errors.New("unsupported service mode"), "invalid_mode", "service mode is not supported")
	}
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
