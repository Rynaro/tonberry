// Package mcpserver wires the 11 tonberry ops onto an official Go MCP SDK stdio
// server (FORGE Decision 5). list_tools returns the 11-tool manifest; call_tool
// name-dispatches to the ops layer. The 11 tools (bare names, maker_checker
// folded into verify as C4):
//
//	propose, right_size, transition, compose_manifest,
//	compose_envelope, verify, drift_check, archive,
//	list, status, assess   (v0.2.0: read-only project-scope observability)
//
// Tool namespace at the host is mcp__tonberry__<name> (the .mcp.json server key
// "tonberry" + the bare tool name); this package registers the bare names.
package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rynaro/tonberry/internal/ops"
)

// ServerName is the MCP server name and the .mcp.json server key.
const ServerName = "tonberry"

// Version is the tonberry build version reported to MCP clients.
const Version = "0.2.0"

// ToolNames is the canonical, ordered list of the 11 tools.
var ToolNames = []string{
	"propose",
	"right_size",
	"transition",
	"compose_manifest",
	"compose_envelope",
	"verify",
	"drift_check",
	"archive",
	"list",
	"status",
	"assess",
}

// resolveAbs resolves a change_dir argument to an absolute path, erroring if the
// folder does not exist or is not a directory (mirrors the bash checker's usage
// pre-checks; the caller maps the error to a usage failure).
func resolveAbs(p string) (string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("change folder not found: %s", p)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// New constructs the MCP server with all 8 tools registered.
func New() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: Version,
		Title:   "tonberry — official ESL (Eidolons Spec Lifecycle) MCP",
	}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose",
		Description: "Scaffold a change.json (status=proposed) under .spectra/changes/<change_id>/. tier is null until right_size.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.ProposeInput) (*mcp.CallToolResult, ops.ProposeOutput, error) {
		out, err := ops.Propose(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "right_size",
		Description: "Deterministic 3-signal ESL §4 gate: (files_touched, rubric_score/12, tradeoff_present) -> trivial/lite/full. Same signals always yield the same tier.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.RightSizeInput) (*mcp.CallToolResult, ops.RightSizeOutput, error) {
		out, err := ops.RightSize(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "transition",
		Description: "Advance status honoring ESL §3 skip-rules (lite/trivial skip deliberated; code-states require code; archived requires drift_checked).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.TransitionInput) (*mcp.CallToolResult, ops.TransitionOutput, error) {
		out, err := ops.Transition(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "compose_manifest",
		Description: "Write/validate change.json against the ESL-owned change.v1.json (references spec_ref; never inlines the SPECTRA schema).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.ComposeManifestInput) (*mcp.CallToolResult, ops.ComposeManifestOutput, error) {
		out, err := ops.ComposeManifest(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "compose_envelope",
		Description: "Emit an ECL sidecar *.envelope.json naming the §7.2 performative for a transition (references the closed-10 set; never re-declares the ECL schema).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.ComposeEnvelopeInput) (*mcp.CallToolResult, ops.ComposeEnvelopeOutput, error) {
		out, err := ops.ComposeEnvelope(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "verify",
		Description: "Run the 6 mechanical ESL conformance checks C1–C6 (incl. maker!=checker as C4). Parity-locked to esl-conformance.sh. mode=warn|block; exit_code 0/3.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.VerifyInput) (*mcp.CallToolResult, ops.VerifyOutput, error) {
		out, err := ops.Verify(in, resolveAbs)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "drift_check",
		Description: "Identity-distinct checker (checker!=maker) records the drift verdict; mismatch -> ESCALATE to in_progress; match -> drift_checked=true (ESL §6.4).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.DriftCheckInput) (*mcp.CallToolResult, ops.DriftCheckOutput, error) {
		out, err := ops.DriftCheck(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "archive",
		Description: "Snapshot folder -> archive/<date>-<change_id>/, set status=archived + archive_path, compose the promotion-intent INFORM envelope. Requires drift_checked=true. Never calls crystalium.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.ArchiveInput) (*mcp.CallToolResult, ops.ArchiveOutput, error) {
		out, err := ops.Archive(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list",
		Description: "Enumerate change folders under .spectra/changes/ (skips the archive/ snapshot subdir); returns [{change_id,status,tier,drift_checked}] sorted by change_id. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.ListInput) (*mcp.CallToolResult, ops.ListOutput, error) {
		out, err := ops.List(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "status",
		Description: "For one change_id: the manifest summary + the verify verdict (the SAME 6 checks) + the legal next lifecycle transitions. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.StatusInput) (*mcp.CallToolResult, ops.StatusOutput, error) {
		out, err := ops.Status(in)
		return result(err), deref(out), err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "assess",
		Description: "Project-scope escalation assessment: aggregate the §4.2 signals (change_count/repo_loc/full_ratio) vs thresholds (N=10/L=50000/R=0.4, overridable) -> recommended_mode advisory|block. Deterministic, read-only; tonberry ships the assessment, the nexus records the flip (eidolons-esl docs/escalation.md).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ops.AssessInput) (*mcp.CallToolResult, ops.AssessOutput, error) {
		out, err := ops.Assess(in)
		return result(err), deref(out), err
	})

	return s
}

// Serve runs the stdio MCP server until the client disconnects or ctx is cancelled.
func Serve(ctx context.Context) error {
	return New().Run(ctx, &mcp.StdioTransport{})
}

// result builds the unstructured CallToolResult. On a tool error we set IsError
// so the model sees the failure and can self-correct (per the SDK guidance:
// tool-origin errors ride the result, not the protocol). The structured output
// (the typed Out) is attached by the SDK from the returned value.
func result(err error) *mcp.CallToolResult {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
		}
	}
	return nil // SDK fills Content from the structured Out when result is nil
}

// deref returns the zero value when out is nil (error path), else *out.
func deref[T any](out *T) T {
	if out == nil {
		var zero T
		return zero
	}
	return *out
}
