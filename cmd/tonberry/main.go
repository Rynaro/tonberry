// Command tonberry is the dual-mode binary for the official ESL MCP (FORGE
// Decision 5):
//
//	tonberry serve                              -> stdio MCP server (the 8 tools)
//	tonberry verify <change-dir> [--mode m] [--json]
//	                                            -> CI/standalone conformance checker
//	                                               (parity-locked to esl-conformance.sh)
//	tonberry <op> [flags...]                    -> one-shot CLI for any op (JSON to stdout)
//	tonberry version | --version | -h | --help
//
// The verify path is parity-critical: exit codes (0/1/3, 2 reserved), --json to
// stdout, human findings to stderr — all must match the vendored bash checker.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Rynaro/tonberry/internal/conformance"
	"github.com/Rynaro/tonberry/internal/manifest"
	"github.com/Rynaro/tonberry/internal/mcpserver"
	"github.com/Rynaro/tonberry/internal/ops"
)

// Version is the tonberry build/release version.
const Version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "serve":
		if err := mcpserver.Serve(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "tonberry serve:", err)
			os.Exit(1)
		}
	case "verify":
		os.Exit(runVerify(args))
	case "propose":
		runOp(args, opPropose)
	case "right_size":
		runOp(args, opRightSize)
	case "transition":
		runOp(args, opTransition)
	case "compose_manifest":
		runOp(args, opComposeManifest)
	case "compose_envelope":
		runOp(args, opComposeEnvelope)
	case "drift_check":
		runOp(args, opDriftCheck)
	case "archive":
		runOp(args, opArchive)
	case "version", "--version":
		fmt.Println(Version)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "tonberry: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `tonberry — official ESL (Eidolons Spec Lifecycle) MCP

Usage:
  tonberry serve
      Run the stdio MCP server (the 8 tonberry tools).

  tonberry verify <change-dir> [--mode warn|block] [--json]
      Run the 6 ESL conformance checks C1–C6 against a change folder.
      Exit: 0 conformant / 1 usage error / 3 hard violation (--mode block).

  tonberry <op> [--key value ...]
      One-shot CLI for any op. JSON result to stdout. Ops:
        propose right_size transition compose_manifest
        compose_envelope drift_check archive

  tonberry version | -h | --help
`)
}

// -- verify (the parity surface; matches esl-conformance.sh exactly) -------- //

func runVerify(args []string) int {
	target := ""
	mode := "warn"
	jsonOut := false

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--mode":
			if i+1 >= len(args) {
				return usageErr("Missing value for --mode")
			}
			mode = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--mode="):
			mode = strings.TrimPrefix(a, "--mode=")
			i++
		case a == "--json":
			jsonOut = true
			i++
		case a == "--strict":
			mode = "block"
			i++
		case a == "-h" || a == "--help":
			usage()
			return 0
		case a == "--":
			i++
		case strings.HasPrefix(a, "-"):
			return usageErr("Unknown option: " + a)
		default:
			if target == "" {
				target = a
			} else {
				return usageErr("Unexpected extra argument: " + a)
			}
			i++
		}
	}

	if mode != "warn" && mode != "block" {
		return usageErr("Invalid --mode: " + mode + " (expected warn or block)")
	}
	if target == "" {
		return usageErr("Missing <change-folder>.")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return usageErr("Change folder not found: " + target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return usageErr("Change folder not found: " + target)
	}

	rep := conformance.Check(abs, conformance.Mode(mode))

	// Human findings -> stderr (deterministic: the order checks were run).
	printHuman(rep)

	// --json machine summary -> stdout ONLY.
	if jsonOut {
		printJSON(rep)
	}
	return rep.ExitCode
}

func usageErr(msg string) int {
	fmt.Fprintln(os.Stderr, msg)
	fmt.Fprintln(os.Stderr, "Usage: tonberry verify <change-folder> [--mode warn|block] [--json]")
	return 1
}

func printHuman(rep conformance.Report) {
	w := os.Stderr
	fmt.Fprintln(w, "ESL conformance check")
	fmt.Fprintln(w, "Target:", rep.TargetBasename)
	fmt.Fprintln(w, "Mode:  ", rep.Mode)
	fmt.Fprintln(w, "----")
	for _, r := range rep.Results {
		tag := "[?]   "
		switch r.Status {
		case "ok":
			tag = "[OK]  "
		case "fail":
			tag = "[FAIL]"
		}
		line := fmt.Sprintf("%s %s %s %s", tag, r.ID, r.Level, r.Name)
		if r.Reason != "" {
			line += " (" + r.Reason + ")"
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w, "----")
	switch rep.ExitCode {
	case 0:
		if rep.HasFail {
			fmt.Fprintln(w, "Result: WARN — violations present, exit 0 (--mode warn)")
		} else {
			fmt.Fprintln(w, "Result: OK (exit 0)")
		}
	case 3:
		fmt.Fprintln(w, "Result: BLOCK — hard violation (exit 3)")
	}
}

// printJSON emits the --json summary to stdout, structurally matching the bash
// checker's shape: {target_basename, mode, results[{id,level,status,name,reason?}], exit_code}.
func printJSON(rep conformance.Report) {
	type jsonResult struct {
		ID     string `json:"id"`
		Level  string `json:"level"`
		Status string `json:"status"`
		Name   string `json:"name"`
		Reason string `json:"reason,omitempty"`
	}
	out := struct {
		TargetBasename string       `json:"target_basename"`
		Mode           string       `json:"mode"`
		Results        []jsonResult `json:"results"`
		ExitCode       int          `json:"exit_code"`
	}{
		TargetBasename: rep.TargetBasename,
		Mode:           rep.Mode,
		ExitCode:       rep.ExitCode,
	}
	for _, r := range rep.Results {
		out.Results = append(out.Results, jsonResult{r.ID, r.Level, r.Status, r.Name, r.Reason})
	}
	if out.Results == nil {
		out.Results = []jsonResult{}
	}
	b, _ := json.Marshal(out)
	fmt.Fprintln(os.Stdout, string(b))
}

// -- one-shot CLI ops (JSON to stdout) -------------------------------------- //

type flagMap map[string]string

// parseFlags turns --key value / --key=value / --bool into a map. A flag with no
// following value (or followed by another --flag) is treated as boolean "true".
func parseFlags(args []string) flagMap {
	fm := flagMap{}
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			i++
			continue
		}
		key := strings.TrimPrefix(a, "--")
		if eq := strings.IndexByte(key, '='); eq >= 0 {
			fm[key[:eq]] = key[eq+1:]
			i++
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			fm[key] = args[i+1]
			i += 2
		} else {
			fm[key] = "true"
			i++
		}
	}
	return fm
}

func (f flagMap) str(k string) string  { return f[k] }
func (f flagMap) boolean(k string) bool { return f[k] == "true" || f[k] == "1" }
func (f flagMap) integer(k string) int {
	n, _ := strconv.Atoi(f[k])
	return n
}

func runOp(args []string, fn func(flagMap) (any, error)) {
	fm := parseFlags(args)
	out, err := fn(fm)
	if err != nil {
		emitJSON(map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	emitJSON(out)
}

func emitJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tonberry: marshal:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, string(b))
}

func opPropose(f flagMap) (any, error) {
	return ops.Propose(ops.ProposeInput{
		ProjectRoot: f.str("project_root"),
		ChangeID:    f.str("change_id"),
		Maker:       f.str("maker"),
		SpecRef:     f.str("spec_ref"),
		Checker:     f.str("checker"),
		CreatedAt:   f.str("created_at"),
		Supersedes:  f.str("supersedes"),
	})
}

func opRightSize(f flagMap) (any, error) {
	return ops.RightSize(ops.RightSizeInput{
		ProjectRoot:     f.str("project_root"),
		ChangeID:        f.str("change_id"),
		FilesTouched:    f.integer("files_touched"),
		RubricScore:     f.integer("rubric_score"),
		TradeoffPresent: f.boolean("tradeoff_present"),
		WriteManifest:   f.boolean("write_manifest"),
	})
}

func opTransition(f flagMap) (any, error) {
	return ops.Transition(ops.TransitionInput{
		ProjectRoot:   f.str("project_root"),
		ChangeID:      f.str("change_id"),
		ToStatus:      f.str("to_status"),
		Actor:         f.str("actor"),
		HasCode:       f.boolean("has_code"),
		WriteManifest: f.boolean("write_manifest"),
	})
}

func opComposeManifest(f flagMap) (any, error) {
	in := ops.ComposeManifestInput{
		ProjectRoot: f.str("project_root"),
		ChangeID:    f.str("change_id"),
	}
	if patch := f.str("patch"); patch != "" {
		var c manifest.Change
		if err := json.Unmarshal([]byte(patch), &c); err != nil {
			return nil, fmt.Errorf("--patch is not valid change JSON: %w", err)
		}
		in.Patch = &c
	}
	return ops.ComposeManifest(in)
}

func opComposeEnvelope(f flagMap) (any, error) {
	in := ops.ComposeEnvelopeInput{
		ProjectRoot:             f.str("project_root"),
		ChangeID:                f.str("change_id"),
		Performative:            f.str("performative"),
		Transition:              f.str("transition"),
		From:                    f.str("from"),
		To:                      f.str("to"),
		Basename:                f.str("basename"),
		Objective:               f.str("objective"),
		EnvelopeVersionOverride: f.str("envelope_version"),
	}
	if cd := f.str("context_delta"); cd != "" {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(cd), &m); err != nil {
			return nil, fmt.Errorf("--context_delta is not valid JSON: %w", err)
		}
		in.ContextDelta = m
	}
	return ops.ComposeEnvelope(in)
}

func opDriftCheck(f flagMap) (any, error) {
	in := ops.DriftCheckInput{
		ProjectRoot:     f.str("project_root"),
		ChangeID:        f.str("change_id"),
		Checker:         f.str("checker"),
		SpecOfRecordRef: f.str("spec_of_record_ref"),
		TreeRoot:        f.str("tree_root"),
		WriteManifest:   f.boolean("write_manifest"),
	}
	if ms := f.str("mismatches"); ms != "" {
		var arr []string
		if err := json.Unmarshal([]byte(ms), &arr); err != nil {
			// allow comma-separated as a convenience
			arr = strings.Split(ms, ",")
		}
		in.Mismatches = arr
	}
	return ops.DriftCheck(in)
}

func opArchive(f flagMap) (any, error) {
	return ops.Archive(ops.ArchiveInput{
		ProjectRoot:             f.str("project_root"),
		ChangeID:                f.str("change_id"),
		Date:                    f.str("date"),
		EnvelopeVersionOverride: f.str("envelope_version"),
	})
}
