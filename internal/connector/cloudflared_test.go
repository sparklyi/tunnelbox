package connector

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sparklyi/tunnelbox/internal/provision"
)

func TestRuntimeQuickTunnelCapturesURLAndUsesSafeArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX test executable")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "cloudflared-fake")
	script := "#!/bin/sh\nprintf 'visit https://preview.trycloudflare.com\\n'\nprintf '%s\\n' \"$@\" > \"$0.args\"\ntrap 'exit 0' INT TERM\nwhile :; do :; done\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake cloudflared: %v", err)
	}

	runtime, err := New(binary, filepath.Join(dir, "state"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = runtime.Close(closeCtx)
	}()

	origin := "http://127.0.0.1:8080"
	if err := runtime.EnsureRunning(context.Background(), provision.ConnectorSpec{ServiceID: "svc_quick", OriginURL: origin, Quick: true}); err != nil {
		t.Fatalf("ensure quick connector: %v", err)
	}
	var status provision.ConnectorStatus
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, err = runtime.Status(context.Background(), "svc_quick")
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if status.URL != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("quick URL was not captured: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.URL != "https://preview.trycloudflare.com" {
		t.Fatalf("captured URL = %q", status.URL)
	}
	args, err := os.ReadFile(binary + ".args")
	if err != nil {
		t.Fatalf("read fake arguments: %v", err)
	}
	if got, want := strings.TrimSpace(string(args)), "tunnel\n--no-autoupdate\n--url\n"+origin; got != want {
		t.Fatalf("arguments = %q, want %q", got, want)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runtime.Stop(stopCtx, "svc_quick"); err != nil {
		t.Fatalf("stop connector: %v", err)
	}
	status, err = runtime.Status(context.Background(), "svc_quick")
	if err != nil {
		t.Fatalf("status after stop: %v", err)
	}
	if status.Running {
		t.Fatalf("connector still running after stop: %+v", status)
	}
}

func TestOutputCaptureAcceptsURLSplitAcrossWrites(t *testing.T) {
	capture := &outputCapture{}
	capture.observe("try https://split.trycloud")
	capture.observe("flare.com now")
	if got := capture.URL(); got != "https://split.trycloudflare.com" {
		t.Fatalf("captured URL = %q", got)
	}
}
