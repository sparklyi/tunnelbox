package connector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sparklyi/tunnelbox/internal/auth"
	"github.com/sparklyi/tunnelbox/internal/provision"
)

type Error struct {
	Code  string
	Cause error
}

func (e *Error) Error() string {
	if e == nil || e.Code == "" {
		return "cloudflared process failed"
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *Error) FailureCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *Error) RemoteStateUnknown() bool { return false }

type Runtime struct {
	binary  string
	dataDir string
	logger  *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc

	mu        sync.Mutex
	processes map[string]*process
}

type process struct {
	cmd       *exec.Cmd
	spec      provision.ConnectorSpec
	output    *outputCapture
	done      chan struct{}
	waitError error
	startedAt time.Time
}

func New(binary, dataDir string, logger *slog.Logger) (*Runtime, error) {
	binary = strings.TrimSpace(binary)
	dataDir = strings.TrimSpace(dataDir)
	if binary == "" || dataDir == "" {
		return nil, errors.New("cloudflared binary and data directory are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tokens"), 0o700); err != nil {
		return nil, fmt.Errorf("create cloudflared data directory: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runtime{binary: binary, dataDir: dataDir, logger: logger, ctx: ctx, cancel: cancel, processes: make(map[string]*process)}, nil
}

func (r *Runtime) EnsureRunning(ctx context.Context, spec provision.ConnectorSpec) error {
	if err := validateSpec(spec); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	if existing, ok := r.processes[spec.ServiceID]; ok {
		select {
		case <-existing.done:
			delete(r.processes, spec.ServiceID)
		default:
			r.mu.Unlock()
			return nil
		}
	}
	args := []string{"tunnel", "--no-autoupdate"}
	if spec.Quick {
		args = append(args, "--url", spec.OriginURL)
	} else {
		tokenPath := filepath.Join(r.dataDir, "tokens", spec.ServiceID+".token")
		if err := auth.SaveToken(tokenPath, spec.Token); err != nil {
			r.mu.Unlock()
			return &Error{Code: "connector_token_file_failed", Cause: err}
		}
		args = append(args, "run", "--token-file", tokenPath)
	}
	capture := &outputCapture{}
	writer := &redactingWriter{logger: r.logger, secret: spec.Token, capture: capture}
	cmd := exec.CommandContext(r.ctx, r.binary, args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		return &Error{Code: "connector_start_failed", Cause: err}
	}
	item := &process{cmd: cmd, spec: spec, output: capture, done: make(chan struct{}), startedAt: time.Now().UTC()}
	r.processes[spec.ServiceID] = item
	r.mu.Unlock()
	go r.wait(spec.ServiceID, item)
	return nil
}

func (r *Runtime) wait(serviceID string, item *process) {
	err := item.cmd.Wait()
	r.mu.Lock()
	item.waitError = err
	close(item.done)
	if current, ok := r.processes[serviceID]; ok && current == item {
		// Keep the completed process until the next status call so its exit reason
		// is visible to the caller.
	}
	r.mu.Unlock()
	if err != nil && !errors.Is(err, context.Canceled) {
		r.logger.Warn("cloudflared exited", "service_id", serviceID, "error", "process exited")
	}
}

func (r *Runtime) Status(ctx context.Context, serviceID string) (provision.ConnectorStatus, error) {
	if err := ctx.Err(); err != nil {
		return provision.ConnectorStatus{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.processes[serviceID]
	if !ok {
		return provision.ConnectorStatus{ServiceID: serviceID, Message: "not running"}, nil
	}
	select {
	case <-item.done:
		delete(r.processes, serviceID)
		return provision.ConnectorStatus{ServiceID: serviceID, Mode: connectorMode(item.spec), URL: item.output.URL(), Message: "process exited"}, nil
	default:
		status := provision.ConnectorStatus{ServiceID: serviceID, Mode: connectorMode(item.spec), Running: true, URL: item.output.URL()}
		status.Healthy = !item.spec.Quick || status.URL != ""
		if status.Healthy {
			status.Message = "process running"
		} else {
			status.Message = "waiting for quick tunnel URL"
		}
		return status, nil
	}
}

func (r *Runtime) List(ctx context.Context) ([]provision.ConnectorStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]provision.ConnectorStatus, 0, len(r.processes))
	for serviceID, item := range r.processes {
		select {
		case <-item.done:
			delete(r.processes, serviceID)
			items = append(items, provision.ConnectorStatus{ServiceID: serviceID, Mode: connectorMode(item.spec), URL: item.output.URL(), Message: "process exited"})
		default:
			status := provision.ConnectorStatus{ServiceID: serviceID, Mode: connectorMode(item.spec), URL: item.output.URL(), Running: true}
			status.Healthy = !item.spec.Quick || status.URL != ""
			if status.Healthy {
				status.Message = "process running"
			} else {
				status.Message = "waiting for quick tunnel URL"
			}
			items = append(items, status)
		}
	}
	sort.Slice(items, func(a, b int) bool { return items[a].ServiceID < items[b].ServiceID })
	return items, nil
}

func (r *Runtime) Reload(ctx context.Context, serviceID string) error {
	r.mu.Lock()
	item, ok := r.processes[serviceID]
	if !ok {
		r.mu.Unlock()
		return &Error{Code: "connector_not_running"}
	}
	spec := item.spec
	r.mu.Unlock()
	if err := r.Stop(ctx, serviceID); err != nil {
		return err
	}
	return r.EnsureRunning(ctx, spec)
}

func (r *Runtime) Stop(ctx context.Context, serviceID string) error {
	r.mu.Lock()
	item, ok := r.processes[serviceID]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	if err := item.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = item.cmd.Process.Kill()
	}
	select {
	case <-item.done:
		r.mu.Lock()
		if current, ok := r.processes[serviceID]; ok && current == item {
			delete(r.processes, serviceID)
		}
		r.mu.Unlock()
		return nil
	case <-ctx.Done():
		_ = item.cmd.Process.Kill()
		return ctx.Err()
	}
}

func (r *Runtime) Close(ctx context.Context) error {
	r.cancel()
	r.mu.Lock()
	items := make([]*process, 0, len(r.processes))
	for _, item := range r.processes {
		items = append(items, item)
	}
	r.mu.Unlock()
	for _, item := range items {
		select {
		case <-item.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func validateSpec(spec provision.ConnectorSpec) error {
	if spec.ServiceID == "" {
		return errors.New("connector service id is required")
	}
	if spec.ServiceID == "." || spec.ServiceID == ".." || strings.ContainsAny(spec.ServiceID, `/\\`) {
		return errors.New("connector service id is invalid")
	}
	if spec.Quick {
		u, err := url.Parse(spec.OriginURL)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("quick tunnel origin must be an http or https URL")
		}
		return nil
	}
	if spec.TunnelID == "" || spec.Token == "" {
		return errors.New("connector tunnel id and token are required")
	}
	return nil
}

type redactingWriter struct {
	logger  *slog.Logger
	secret  string
	capture *outputCapture
}

func (w *redactingWriter) Write(data []byte) (int, error) {
	message := string(data)
	if w.secret != "" {
		message = strings.ReplaceAll(message, w.secret, "[redacted]")
	}
	if w.capture != nil {
		w.capture.observe(message)
	}
	if w.logger == nil {
		return len(data), nil
	}
	if strings.TrimSpace(message) != "" {
		w.logger.Info("cloudflared output", "line", strings.TrimSpace(message))
	}
	return len(data), nil
}

type outputCapture struct {
	mu      sync.RWMutex
	url     string
	pending string
}

var quickTunnelURL = regexp.MustCompile(`https://[A-Za-z0-9][A-Za-z0-9.-]*\.trycloudflare\.com`)

func (c *outputCapture) observe(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	combined := c.pending + message
	if len(combined) > 16*1024 {
		combined = combined[len(combined)-(16*1024):]
	}
	c.pending = combined
	match := quickTunnelURL.FindString(combined)
	if match == "" {
		return
	}
	match = strings.TrimRight(match, ".,;:)]}")
	u, err := url.Parse(match)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return
	}
	c.url = u.String()
}

func (c *outputCapture) URL() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.url
}

func connectorMode(spec provision.ConnectorSpec) string {
	if spec.Quick {
		return "quick"
	}
	return "managed"
}

var _ io.Writer = (*redactingWriter)(nil)
var _ provision.ConnectorRuntime = (*Runtime)(nil)
var _ provision.ConnectorLister = (*Runtime)(nil)
