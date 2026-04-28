package handlers

import (
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/shapeup"
)

// resolveParentProjects implements the Phase 4 default-fill rule for
// downstream artifact creation. When the request supplies an explicit
// ProjectIDs list, it wins (the user knows what they want). When empty,
// the parent's `Projects` field fills in.
//
// All values are funneled through CanonicalizeProjectIDs so the on-disk
// wire format stays UUID-only and dangling refs drop.
//
// supplied: ProjectIDs taken straight off the request body.
// parent:   Projects field of the parent artifact (Pitch thread for a PRD,
//
//	Breadboard thread for a Prototype, PRD for a UseCase). Pass
//	nil when there is no resolvable parent.
func resolveParentProjects(s *projects.Store, supplied, parent []string) []string {
	if len(supplied) > 0 {
		return projects.CanonicalizeProjectIDs(s, supplied)
	}
	if len(parent) > 0 {
		return projects.CanonicalizeProjectIDs(s, parent)
	}
	return nil
}

// findThreadByArtifactPath returns the first thread containing an
// artifact whose Path matches `wantPath`. Used to look up a Pitch's
// owning thread when the FE only sends pitch_path (not source_thread).
// Returns nil on miss.
func findThreadByArtifactPath(store *shapeup.Store, wantPath string) *shapeup.Thread {
	if store == nil || wantPath == "" {
		return nil
	}
	threads, err := store.ListThreads()
	if err != nil {
		return nil
	}
	for _, t := range threads {
		for _, a := range t.Artifacts {
			if a.Path == wantPath {
				return t
			}
		}
	}
	return nil
}
