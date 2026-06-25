# Archived snapshot — full-new-subsystem (2026-06-24)

On `archived`, IDG snapshots the whole change folder here under
`archive/<date>-<change_id>/` so the active `.spectra/changes/` surface stays
small and the history stays auditable on disk.

Complementing this on-disk snapshot, the consumer Eidolon promotes the verified
living spec to the **CRYSTALIUM Semantic layer** (the durable spec-of-record).
That `mcp__crystalium__commit` (Semantic) call is the consumer Eidolon's, not
ESL's — ESL only DEFINES the archive transition (ESL v1.0 §6). A later change
that alters this behavior sets `change.json.supersedes` and revises the Semantic
record via bi-temporal `update` (never hard-delete).
