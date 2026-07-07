// Package archive snapshots a verified change folder into
// archive/<date>-<change_id>/, sets archive_path + status=archived on the
// manifest, and composes a promotion-intent ECL envelope on disk (FORGE Decision 4).
//
// GAP-D RESOLUTION (compose, don't call): tonberry NEVER imports, links, or calls
// CRYSTALIUM. On archive it writes a promotion-intent INFORM envelope alongside
// the archived folder; the parent/cortex (which holds both grants) routes that
// sidecar to mcp__crystalium__ingest. The promotion intent rides the envelope's
// context_delta FIELD (ESL §7.3) — there is NO ACCEPT performative; the
// archive/promotion transition is ACKNOWLEDGE + INFORM(promotion) (ESL §7.2/§7.3,
// esl-1.0.md:148-150). The envelope REFERENCES spec_ref + change_id; it does NOT
// embed a CRYSTALIUM commit/ingest payload shape (anti-scope, ESL §1.3).
package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Rynaro/tonberry/internal/envelope"
	"github.com/Rynaro/tonberry/internal/fsdiag"
	"github.com/Rynaro/tonberry/internal/manifest"
)

// PromotionEnvelopeName is the on-disk promotion-intent sidecar filename.
const PromotionEnvelopeName = "promotion.envelope.json"

// Result is the outcome of an archive operation.
type Result struct {
	ArchivePath           string `json:"archive_path"`
	Status                string `json:"status"`
	PromotionEnvelopePath string `json:"promotion_envelope_path"`
	PromotionPerformative string `json:"promotion_performative"`
}

// Archive MOVES changeDir to <changesRoot>/archive/<date>-<change_id>/ (ESL §9.2:
// a move, not a copy — the active .spectra/changes/<change_id>/ no longer exists
// afterward), sets status=archived + archive_path on the moved change.json, and
// composes the promotion-intent INFORM envelope inside the moved folder.
//
// Precondition (ESL §6.4, conformance C5): drift_checked MUST be true. If not,
// Archive refuses (this is the graceful pre-check that mirrors the C5 block).
//
// date defaults to today (UTC, YYYY-MM-DD) when empty. The archive root is
// derived from the change folder's parent: a change folder lives at
// .spectra/changes/<change_id>/, and it is moved to
// .spectra/changes/archive/<date>-<change_id>/ — keeping everything under
// .spectra/ (ESL §9.2 "MUST NOT introduce a new top-level consumer directory").
func Archive(changeDir, date string, envVersionOverride string) (*Result, error) {
	c, err := manifest.Read(changeDir)
	if err != nil {
		return nil, err
	}
	if !c.DriftCheckedTrue() {
		return nil, fmt.Errorf("cannot archive %q: drift_checked must be true before archive (ESL §6.4)", c.ChangeID)
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	// archive root sits alongside the changes folders, under .spectra/.
	changesRoot := filepath.Dir(changeDir) // .spectra/changes
	archiveRoot := filepath.Join(changesRoot, "archive")
	snapName := fmt.Sprintf("%s-%s", date, c.ChangeID)
	snapDir := filepath.Join(archiveRoot, snapName)
	relArchivePath := filepath.ToSlash(filepath.Join("archive", snapName))

	// MOVE the change folder into archive/<date>-<change_id>/ (ESL §9.2). Rename is
	// atomic within a filesystem; fall back to copy+remove only if Rename fails
	// (e.g. a cross-device move). The active change folder must not survive.
	if err := os.MkdirAll(archiveRoot, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir archive root %s: %w", archiveRoot, fsdiag.Explain(err, archiveRoot))
	}
	if _, err := os.Stat(snapDir); err == nil {
		return nil, fmt.Errorf("archive target already exists: %s", snapDir)
	}
	if err := os.Rename(changeDir, snapDir); err != nil {
		// Cross-device or other rename failure: copy then remove the original so
		// the end state is still a move (active folder gone).
		if cerr := copyTree(changeDir, snapDir); cerr != nil {
			return nil, fmt.Errorf("move %s -> %s: rename failed (%v) and copy fallback failed: %w", changeDir, snapDir, err, cerr)
		}
		if rerr := os.RemoveAll(changeDir); rerr != nil {
			return nil, fmt.Errorf("move %s -> %s: copied but could not remove original: %w", changeDir, snapDir, rerr)
		}
	}

	// Update the moved folder's manifest: status=archived, archive_path set.
	c.Status = manifest.StatusArchived
	c.ArchivePath = &relArchivePath
	if _, err := manifest.Write(snapDir, c); err != nil {
		return nil, err
	}

	// Compose the promotion-intent envelope inside the moved folder.
	specRef := "null"
	if c.SpecRef != nil {
		specRef = *c.SpecRef
	}
	env, err := envelope.Compose("INFORM", "idg", "orchestrator", envelope.Options{
		EnvelopeVersionOverride: envVersionOverride,
		Objective:               fmt.Sprintf("INFORM(promotion): archived change %q ready for CRYSTALIUM Semantic promotion.", c.ChangeID),
		ContextDelta: map[string]interface{}{
			"intent":       "promotion",
			"change_id":    c.ChangeID,
			"spec_ref":     specRef,
			"archive_path": relArchivePath,
			"summary":      "Verified + archived spec; route to mcp__crystalium__ingest (Semantic layer). tonberry does not call crystalium.",
		},
	})
	if err != nil {
		return nil, err
	}
	promoPath, err := envelope.Write(snapDir, PromotionEnvelopeName, env)
	if err != nil {
		return nil, err
	}

	return &Result{
		ArchivePath:           relArchivePath,
		Status:                string(manifest.StatusArchived),
		PromotionEnvelopePath: promoPath,
		PromotionPerformative: "INFORM",
	}, nil
}

// copyTree recursively copies src into dst (files + subdirs), skipping a nested
// pre-existing archive/ directory to avoid recursive self-inclusion.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode())
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() == "archive" {
			continue // don't recurse into a sibling archive dir
		}
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		ei, ierr := e.Info()
		if ierr != nil {
			return ierr
		}
		if e.IsDir() {
			if err := copyTree(s, d); err != nil {
				return err
			}
		} else if ei.Mode().IsRegular() {
			if err := copyFile(s, d, ei.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
