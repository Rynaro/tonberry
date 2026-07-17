// Command tonberry is the dual-mode binary for the official ESL MCP (FORGE
// Decision 5):
//
//	tonberry serve                              -> stdio MCP server (the 11 tools)
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
const Version = "0.5.3"

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
	case "list":
		runOpArgs(args, opList)
	case "status":
		runOpArgs(args, opStatus)
	case "assess":
		runOpArgs(args, opAssess)
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
      Run the stdio MCP server (the 11 tonberry tools).

  tonberry verify <change-dir> [--mode warn|block] [--json]
      Run the 6 MUST ESL conformance checks C1–C6 plus the SHOULD advisory
      checks C7 (EARS lint) and C8 (fresh-context attestation) against a
      change folder. C7/C8 never affect the exit code.
      Exit: 0 conformant / 1 usage error / 3 hard violation (--mode block).

  tonberry <op> [--key value ...]
      One-shot CLI for any op. JSON result to stdout. Ops:
        propose right_size transition compose_manifest
        compose_envelope drift_check archive
        list status assess

      transition / right_size / drift_check PERSIST the manifest by default
      when the action is allowed/valid (v0.4.0 flip). Pass --dry-run to
      evaluate-only (the old behavior); --write_manifest still works.
      propose accepts --has_code (bool); transition READS has_code from the
      manifest (override with --has_code). archive MOVES the change folder
      into archive/ (the active folder no longer exists afterward).
      propose accepts --memory_preflight_ran (bool) + --memory_preflight_records
      (int), given together, to persist the OPTIONAL v1.1 recall-before-
      authoring record (omit both to skip it; still conformant).

  tonberry list   [DIR] [--changes_dir DIR] [--project_root DIR] [--all]
      Enumerate ACTIVE change folders:
      [{change_id,status,tier,drift_checked,archived}].
      DIR is the changes dir (default .spectra/changes); positional and
      --changes_dir are equivalent (--changes_dir wins if both are given).
      --all (alias --include-archived) ALSO lists archived snapshots.

  tonberry status [DIR] [--changes_dir DIR] --change_id ID
                  [--project_root DIR] [--mode warn|block]
      Manifest summary + verify verdict + legal next transitions.
      DIR / --changes_dir locate the changes dir (default .spectra/changes).

  tonberry assess [DIR] [--changes_dir DIR] [--project_root DIR]
                  [--repo_loc N] [--n N] [--l L] [--r R]
      Project-scope escalation assessment (change_count/repo_loc/full_ratio
      vs thresholds N=10/L=50000/R=0.4) -> recommended_mode advisory|block.
      DIR / --changes_dir locate the changes dir (default .spectra/changes).

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

// firstPositional returns the first non-flag, non-flag-value token in args, or ""
// if there is none. It mirrors parseFlags' rule that a flag is consumed with its
// following value (unless that value itself starts with "--"), so a value like
// `--mode warn` does not leak "warn" as a positional.
func firstPositional(args []string) string {
	i := 0
	for i < len(args) {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			// `--key=value` consumes one token; `--key value` consumes two
			// (matching parseFlags). A trailing flag or `--key --next` consumes one.
			if strings.IndexByte(a, '=') >= 0 {
				i++
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i += 2
				continue
			}
			i++
			continue
		}
		return a
	}
	return ""
}

// changesDirArg resolves the changes directory for the consistent positional /
// --changes_dir convention shared by list/status/assess: an explicit
// --changes_dir flag wins; otherwise the first positional path is used; the op's
// own default (.spectra/changes, relative to --project_root) applies when neither
// is given. Returns "" when neither a flag nor a positional is present, so the
// op layer keeps owning the default.
func changesDirArg(f flagMap, args []string) string {
	if v := f.str("changes_dir"); v != "" {
		return v
	}
	return firstPositional(args)
}

func (f flagMap) str(k string) string   { return f[k] }
func (f flagMap) boolean(k string) bool { return f[k] == "true" || f[k] == "1" }

// has reports whether a flag key was present at all (any value).
func (f flagMap) has(k string) bool { _, ok := f[k]; return ok }

// boolPtr returns a *bool: nil when the flag is absent (so "unset" is distinct
// from an explicit false), else a pointer to the parsed boolean. A bare flag
// (e.g. `--has_code`) is "true"; `--has_code=false` / `--has_code 0` are false.
func (f flagMap) boolPtr(k string) *bool {
	if !f.has(k) {
		return nil
	}
	v := f.boolean(k)
	return &v
}
func (f flagMap) integer(k string) int {
	n, _ := strconv.Atoi(f[k])
	return n
}

// intPtr returns a *int: nil when the flag is absent (so "unset" is distinct
// from an explicit 0), else a pointer to the parsed integer.
func (f flagMap) intPtr(k string) *int {
	if !f.has(k) {
		return nil
	}
	n := f.integer(k)
	return &n
}
func (f flagMap) float(k string) float64 {
	v, _ := strconv.ParseFloat(f[k], 64)
	return v
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

// runOpArgs is runOp for ops that also accept a positional path (list/status/
// assess): the handler receives both the parsed flag map and the raw args so it
// can resolve the consistent positional / --changes_dir convention.
func runOpArgs(args []string, fn func(flagMap, []string) (any, error)) {
	fm := parseFlags(args)
	out, err := fn(fm, args)
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
		ProjectRoot:            f.str("project_root"),
		ChangeID:               f.str("change_id"),
		Maker:                  f.str("maker"),
		SpecRef:                f.str("spec_ref"),
		Checker:                f.str("checker"),
		CreatedAt:              f.str("created_at"),
		Supersedes:             f.str("supersedes"),
		HasCode:                f.boolPtr("has_code"),
		MemoryPreflightRan:     f.boolPtr("memory_preflight_ran"),
		MemoryPreflightRecords: f.intPtr("memory_preflight_records"),
	})
}

func opRightSize(f flagMap) (any, error) {
	return ops.RightSize(ops.RightSizeInput{
		ProjectRoot:     f.str("project_root"),
		ChangeID:        f.str("change_id"),
		FilesTouched:    f.integer("files_touched"),
		RubricScore:     f.integer("rubric_score"),
		TradeoffPresent: f.boolean("tradeoff_present"),
		WriteManifest:   f.boolPtr("write_manifest"),
		DryRun:          f.boolean("dry_run") || f.boolean("dry-run"),
	})
}

func opTransition(f flagMap) (any, error) {
	return ops.Transition(ops.TransitionInput{
		ProjectRoot:   f.str("project_root"),
		ChangeID:      f.str("change_id"),
		ToStatus:      f.str("to_status"),
		Actor:         f.str("actor"),
		HasCode:       f.boolPtr("has_code"),
		WriteManifest: f.boolPtr("write_manifest"),
		DryRun:        f.boolean("dry_run") || f.boolean("dry-run"),
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
		WriteManifest:   f.boolPtr("write_manifest"),
		DryRun:          f.boolean("dry_run") || f.boolean("dry-run"),
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

// opList, opStatus, opAssess share the positional / --changes_dir convention:
// `tonberry <op> <dir>` and `tonberry <op> --changes_dir <dir>` are equivalent;
// --changes_dir wins if both are given; the op default (.spectra/changes) applies
// when neither is present.
func opList(f flagMap, args []string) (any, error) {
	return ops.List(ops.ListInput{
		ChangesDir:      changesDirArg(f, args),
		ProjectRoot:     f.str("project_root"),
		IncludeArchived: f.boolean("all") || f.boolean("include_archived") || f.boolean("include-archived"),
	})
}

func opStatus(f flagMap, args []string) (any, error) {
	return ops.Status(ops.StatusInput{
		ProjectRoot: f.str("project_root"),
		ChangesDir:  changesDirArg(f, args),
		ChangeID:    f.str("change_id"),
		Mode:        f.str("mode"),
		HasCode:     f.boolean("has_code"),
	})
}

func opAssess(f flagMap, args []string) (any, error) {
	in := ops.AssessInput{
		ProjectRoot: f.str("project_root"),
		ChangesDir:  changesDirArg(f, args),
		RepoLOC:     f.integer("repo_loc"),
		N:           f.integer("n"),
		L:           f.integer("l"),
		R:           f.float("r"),
	}
	return ops.Assess(in)
}
