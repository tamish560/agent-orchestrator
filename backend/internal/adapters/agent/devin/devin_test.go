package devin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestManifest(t *testing.T) {
	m := (&Plugin{}).Manifest()
	if m.ID != "devin" {
		t.Fatalf("ID = %q, want devin", m.ID)
	}
	if m.Name != "Devin" {
		t.Fatalf("Name = %q", m.Name)
	}
	hasAgent := false
	for _, c := range m.Capabilities {
		if c == adapters.CapabilityAgent {
			hasAgent = true
		}
	}
	if !hasAgent {
		t.Fatal("missing CapabilityAgent")
	}
}

func TestGetConfigSpecEmpty(t *testing.T) {
	spec, err := (&Plugin{}).GetConfigSpec(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(spec.Fields) != 0 {
		t.Fatalf("expected no fields, got %d", len(spec.Fields))
	}
}

func TestGetConfigSpecCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Plugin{}).GetConfigSpec(ctx); err == nil {
		t.Fatal("expected ctx error, got nil")
	}
}

func TestGetPromptDeliveryStrategy(t *testing.T) {
	s, err := (&Plugin{}).GetPromptDeliveryStrategy(context.Background(), ports.LaunchConfig{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if s != ports.PromptDeliveryAfterStart {
		t.Fatalf("strategy = %q, want after_start", s)
	}
}

func TestGetLaunchCommandBypass(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	cmd, err := plugin.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Prompt:      "do the thing",
		Permissions: ports.PermissionModeBypassPermissions,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"devin", "--permission-mode", "dangerous"}
	if !reflect.DeepEqual(cmd, want) {
		t.Fatalf("cmd = %#v, want %#v", cmd, want)
	}
}

func TestGetLaunchCommandDefaultPerms(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	cmd, err := plugin.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Prompt: "fix it",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"devin"}
	if !reflect.DeepEqual(cmd, want) {
		t.Fatalf("cmd = %#v, want %#v", cmd, want)
	}
	if strings.Contains(strings.Join(cmd, " "), "permission-mode") {
		t.Fatal("should not have --permission-mode for default perms")
	}
	if strings.Contains(strings.Join(cmd, " "), "-p") {
		t.Fatal("should not use Devin print mode for prompted launches")
	}
}

func TestGetLaunchCommandAcceptEdits(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	cmd, err := plugin.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Prompt:      "refactor auth",
		Permissions: ports.PermissionModeAcceptEdits,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"devin", "--permission-mode", "auto"}
	if !reflect.DeepEqual(cmd, want) {
		t.Fatalf("cmd = %#v, want %#v", cmd, want)
	}
}

func TestGetLaunchCommandAuto(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	cmd, err := plugin.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Prompt:      "ship it",
		Permissions: ports.PermissionModeAuto,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"devin", "--permission-mode", "auto"}
	if !reflect.DeepEqual(cmd, want) {
		t.Fatalf("cmd = %#v, want %#v", cmd, want)
	}
}

func TestGetLaunchCommandNoPrompt(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	cmd, err := plugin.GetLaunchCommand(context.Background(), ports.LaunchConfig{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"devin"}
	if !reflect.DeepEqual(cmd, want) {
		t.Fatalf("cmd = %#v, want %#v", cmd, want)
	}
}

func TestGetLaunchCommandCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Plugin{}).GetLaunchCommand(ctx, ports.LaunchConfig{Prompt: "x"}); err == nil {
		t.Fatal("expected ctx error, got nil")
	}
}

func TestGetRestoreCommand(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	cmd, ok, err := plugin.GetRestoreCommand(context.Background(), ports.RestoreConfig{
		Session: ports.SessionRef{
			Metadata: map[string]string{
				ports.MetadataKeyAgentSessionID: "sess-abc123",
			},
		},
		Permissions: ports.PermissionModeBypassPermissions,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want true")
	}
	want := []string{"devin", "--permission-mode", "dangerous", "-r", "sess-abc123"}
	if !reflect.DeepEqual(cmd, want) {
		t.Fatalf("cmd = %#v, want %#v", cmd, want)
	}
}

func TestGetRestoreCommandNoID(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	_, ok, err := plugin.GetRestoreCommand(context.Background(), ports.RestoreConfig{
		Session: ports.SessionRef{Metadata: map[string]string{}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("ok=true with no agentSessionId, want false")
	}
}

func TestGetRestoreCommandWhitespaceID(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	_, ok, err := plugin.GetRestoreCommand(context.Background(), ports.RestoreConfig{
		Session: ports.SessionRef{Metadata: map[string]string{
			ports.MetadataKeyAgentSessionID: "   ",
		}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("ok=true with whitespace agentSessionId, want false")
	}
}

func TestSessionInfoReadsHookMetadata(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	info, ok, err := plugin.SessionInfo(context.Background(), ports.SessionRef{
		Metadata: map[string]string{
			ports.MetadataKeyAgentSessionID: "devin-ses-1",
			ports.MetadataKeyTitle:          "Fix login redirect",
			ports.MetadataKeySummary:        "Updated the auth callback and tests.",
		},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want true")
	}
	if info.AgentSessionID != "devin-ses-1" {
		t.Fatalf("AgentSessionID = %q, want devin-ses-1", info.AgentSessionID)
	}
	if info.Title != "Fix login redirect" {
		t.Fatalf("Title = %q", info.Title)
	}
	if info.Summary != "Updated the auth callback and tests." {
		t.Fatalf("Summary = %q", info.Summary)
	}
}

func TestSessionInfoFalseWhenNoHookMetadata(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	info, ok, err := plugin.SessionInfo(context.Background(), ports.SessionRef{
		Metadata: map[string]string{},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatalf("ok=true with empty metadata, want false")
	}
	if !reflect.DeepEqual(info, ports.SessionInfo{}) {
		t.Fatalf("info = %#v, want zero", info)
	}
}

func TestGetAgentHooksInstallsLocalDevinConfig(t *testing.T) {
	plugin := &Plugin{resolvedBinary: "devin"}
	ws := t.TempDir()
	if err := plugin.GetAgentHooks(context.Background(), ports.WorkspaceHookConfig{
		WorkspacePath: ws,
		SessionID:     "devin-test-1",
	}); err != nil {
		t.Fatalf("GetAgentHooks: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(ws, ".devin", "config.local.json"))
	if err != nil {
		t.Fatalf("read config.local.json: %v", err)
	}
	var config struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse config.local.json: %v\n%s", err, data)
	}
	sessionStart := config.Hooks["SessionStart"]
	if len(sessionStart) != 1 || len(sessionStart[0].Hooks) != 1 {
		t.Fatalf("SessionStart hooks = %#v, want one AO command", sessionStart)
	}
	hook := sessionStart[0].Hooks[0]
	if hook.Type != "command" || hook.Command != "ao hooks devin session-start" || hook.Timeout != 30 {
		t.Fatalf("SessionStart hook = %#v", hook)
	}
	gitignore, err := os.ReadFile(filepath.Join(ws, ".devin", ".gitignore"))
	if err != nil {
		t.Fatalf("read .devin/.gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), "config.local.json") {
		t.Fatalf(".devin/.gitignore does not ignore config.local.json:\n%s", gitignore)
	}
}

func TestGetAgentHooksCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&Plugin{}).GetAgentHooks(ctx, ports.WorkspaceHookConfig{}); err == nil {
		t.Fatal("expected ctx error, got nil")
	}
}

func TestResolveDevinBinaryFallback(t *testing.T) {
	// When the binary is not on PATH or any well-known location, the resolver
	// MUST surface ports.ErrAgentBinaryNotFound rather than a silent string
	// fallback that lets a missing CLI launch into an empty tmux pane.
	bin, err := ResolveDevinBinary(context.Background())
	if err != nil {
		if !errors.Is(err, ports.ErrAgentBinaryNotFound) {
			t.Fatalf("err = %v, want ports.ErrAgentBinaryNotFound", err)
		}
		return
	}
	if bin == "" {
		t.Fatal("ResolveDevinBinary returned empty path with no error")
	}
}

func TestResolveDevinBinaryCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResolveDevinBinary(ctx); err == nil {
		t.Fatal("expected ctx error, got nil")
	}
}
