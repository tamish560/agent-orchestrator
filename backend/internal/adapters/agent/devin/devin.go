// Package devin implements the Devin ("Devin for Terminal", Cognition) agent
// adapter.
//
// Devin for Terminal (binary "devin") is Cognition's terminal coding agent. It
// has a documented lifecycle hook system. AO installs a local-only Devin hook
// config in `.devin/config.local.json` so Devin can inject AO's generated
// standing instructions from $AO_DATA_DIR/prompts/$AO_SESSION_ID/system.md at
// SessionStart without writing the prompt body into the worktree.
//
// Launch uses `-p <prompt>` for the initial task in non-interactive/print mode
// (in-command delivery). Permission handling uses `--permission-mode`, whose
// valid values are `normal` (aliases: auto) and `dangerous` (aliases: yolo,
// bypass). AO's four permission modes are mapped onto these two: Default emits
// no flag (defer to the user's ~/.config/devin/config.json), AcceptEdits/Auto
// map to `auto`, and BypassPermissions maps to `dangerous`.
//
// Restore prefers a native session id from AO session metadata via `-r <id>`
// when one is available.
package devin

import (
	"context"
	"strings"
	"sync"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/agentbase"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/binaryutil"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

var devinBinarySpec = binaryutil.BinarySpec{
	Label:         "devin",
	Names:         []string{"devin"},
	WinNames:      []string{"devin.cmd", "devin.exe", "devin"},
	UnixPaths:     []string{"/usr/local/bin/devin", "/opt/homebrew/bin/devin"},
	UnixHomePaths: [][]string{{".devin", "bin", "devin"}, {".local", "bin", "devin"}},
	WinPaths: []binaryutil.WinPath{
		{Base: binaryutil.WinHome, Parts: []string{".devin", "bin", "devin.exe"}},
	},
}

// Plugin is the Devin for Terminal agent adapter.
type Plugin struct {
	agentbase.Base
	binaryMu       sync.Mutex
	resolvedBinary string
}

// New returns a ready-to-register Devin adapter.
func New() *Plugin {
	return &Plugin{}
}

var _ adapters.Adapter = (*Plugin)(nil)
var _ ports.Agent = (*Plugin)(nil)

// Manifest returns the adapter's static self-description.
func (p *Plugin) Manifest() adapters.Manifest {
	return adapters.Manifest{
		ID:          "devin",
		Name:        "Devin",
		Description: "Run Cognition Devin for Terminal worker sessions.",
		Version:     "0.0.1",
		Capabilities: []adapters.Capability{
			adapters.CapabilityAgent,
		},
	}
}

// GetLaunchCommand builds `devin [--permission-mode <mode>] -p <prompt>`.
// Prompt is delivered via -p (in command, non-interactive print mode).
//
// Permission values come from `devin --permission-mode -h`:
// `normal` (alias auto) and `dangerous` (aliases yolo, bypass). Default omits
// the flag so Devin uses its config (default mode is auto/normal).
func (p *Plugin) GetLaunchCommand(ctx context.Context, cfg ports.LaunchConfig) (cmd []string, err error) {
	binary, err := p.devinBinary(ctx)
	if err != nil {
		return nil, err
	}

	cmd = []string{binary}
	appendApprovalFlags(&cmd, cfg.Permissions)

	if cfg.Prompt != "" {
		cmd = append(cmd, "-p", cfg.Prompt)
	}

	return cmd, nil
}

func (p *Plugin) GetAgentHooks(ctx context.Context, cfg ports.WorkspaceHookConfig) error {
	return devinHooks.Install(ctx, cfg.WorkspacePath)
}

// GetRestoreCommand builds `devin [--permission-mode <mode>] -r <agentSessionId>`
// when we have a hook-captured native id. ok=false otherwise (fall back to fresh
// launch in the manager).
func (p *Plugin) GetRestoreCommand(ctx context.Context, cfg ports.RestoreConfig) (cmd []string, ok bool, err error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	agentSessionID := strings.TrimSpace(cfg.Session.Metadata[ports.MetadataKeyAgentSessionID])
	if agentSessionID == "" {
		return nil, false, nil
	}

	binary, err := p.devinBinary(ctx)
	if err != nil {
		return nil, false, err
	}

	cmd = make([]string, 0, 5)
	cmd = append(cmd, binary)
	appendApprovalFlags(&cmd, cfg.Permissions)
	cmd = append(cmd, "-r", agentSessionID)
	return cmd, true, nil
}

// SessionInfo reads metadata under AO's normalized keys
// ("title", "summary", "agentSessionId").
func (p *Plugin) SessionInfo(ctx context.Context, session ports.SessionRef) (ports.SessionInfo, bool, error) {
	if err := ctx.Err(); err != nil {
		return ports.SessionInfo{}, false, err
	}
	info, ok := agentbase.StandardSessionInfo(session)
	return info, ok, nil
}

// ResolveDevinBinary finds the `devin` binary (Cognition Devin for Terminal CLI).
func ResolveDevinBinary(ctx context.Context) (string, error) {
	return binaryutil.ResolveBinary(ctx, devinBinarySpec)
}

func (p *Plugin) devinBinary(ctx context.Context) (string, error) {
	p.binaryMu.Lock()
	defer p.binaryMu.Unlock()

	if p.resolvedBinary != "" {
		return p.resolvedBinary, nil
	}

	binary, err := ResolveDevinBinary(ctx)
	if err != nil {
		return "", err
	}
	p.resolvedBinary = binary
	return binary, nil
}

// appendApprovalFlags maps AO's four permission modes onto Devin's two native
// permission values (`auto`/normal and `dangerous`/bypass), per
// `devin --permission-mode -h`.
func appendApprovalFlags(cmd *[]string, permissions ports.PermissionMode) {
	switch ports.NormalizePermissionMode(permissions) {
	case ports.PermissionModeDefault:
		// No flag: defer to ~/.config/devin/config.json (default mode is auto).
	case ports.PermissionModeAcceptEdits:
		// Devin has no dedicated accept-edits flag; auto prompts for writes,
		// which is the safest non-default mapping.
		*cmd = append(*cmd, "--permission-mode", "auto")
	case ports.PermissionModeAuto:
		*cmd = append(*cmd, "--permission-mode", "auto")
	case ports.PermissionModeBypassPermissions:
		*cmd = append(*cmd, "--permission-mode", "dangerous")
	}
}
