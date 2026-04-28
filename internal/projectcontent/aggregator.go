// Package projectcontent fans out the entity stores into a per-project
// view, powering the GET /api/projects/{id}/content endpoint and the
// /projects/:id detail page.
//
// Project membership is derived from each entity's `Projects` field
// (after Phase 1 migration these contain UUIDs). Pre-migration slug
// references are resolved through the projects.Store so a single
// project surfaces consistently.
package projectcontent

import (
	"strings"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/documents"
	"github.com/kriswong/corticalstack/internal/meetings"
	"github.com/kriswong/corticalstack/internal/prds"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/shapeup"
	"github.com/kriswong/corticalstack/internal/usecases"
)

// Counts is a numeric summary used by the Overview tab.
type Counts struct {
	Actions    int `json:"actions"`
	PRDs       int `json:"prds"`
	Prototypes int `json:"prototypes"`
	UseCases   int `json:"usecases"`
	Threads    int `json:"threads"`
	Documents  int `json:"documents"`
	Meetings   int `json:"meetings"`
}

// Content is the per-project payload returned by the API. The Project
// field is the canonical record (carries UUID + slug). Each entity slice
// holds only items that reference this project (by UUID or by slug
// resolving to this project's UUID).
type Content struct {
	Project    *projects.Project       `json:"project"`
	Counts     Counts                  `json:"counts"`
	Actions    []*actions.Action       `json:"actions"`
	PRDs       []*prds.PRD             `json:"prds"`
	Prototypes []*prototypes.Prototype `json:"prototypes"`
	UseCases   []*usecases.UseCase     `json:"usecases"`
	Threads    []*shapeup.Thread       `json:"threads"`
	Documents  []*documents.Document   `json:"documents"`
	Meetings   []*meetings.Meeting     `json:"meetings"`
	// Warnings collects per-store fetch failures (degraded mode). The
	// Content is still returned; the FE renders a banner per warning.
	Warnings []string `json:"warnings,omitempty"`
}

// Aggregator fans out across the entity stores. Wire it once at startup
// like the dashboard aggregator.
type Aggregator struct {
	projects   *projects.Store
	actions    *actions.Store
	prds       *prds.Store
	prototypes *prototypes.Store
	usecases   *usecases.Store
	shapeup    *shapeup.Store
	documents  *documents.Store
	meetings   *meetings.Store
}

// New wires an aggregator. The projects store is required; everything
// else is optional (nil store renders an empty list for that type so
// the FE can still draw the page during partial wiring).
func New(
	p *projects.Store,
	a *actions.Store,
	pr *prds.Store,
	pt *prototypes.Store,
	uc *usecases.Store,
	su *shapeup.Store,
	d *documents.Store,
	m *meetings.Store,
) *Aggregator {
	return &Aggregator{
		projects:   p,
		actions:    a,
		prds:       pr,
		prototypes: pt,
		usecases:   uc,
		shapeup:    su,
		documents:  d,
		meetings:   m,
	}
}

// ForProject returns every entity referencing the given project, looked
// up by UUID or slug. Returns (nil, false) if no project matches.
//
// Membership matching: post-migration values are UUIDs and resolve
// directly. Pre-migration slug values resolve through projects.Store —
// any reference whose canonical UUID matches the target counts.
func (a *Aggregator) ForProject(idOrSlug string) (*Content, bool) {
	if a == nil || a.projects == nil {
		return nil, false
	}
	target := a.projects.Get(idOrSlug)
	if target == nil {
		return nil, false
	}

	c := &Content{Project: target}

	// matches reports whether `refs` contains either the target UUID, or
	// a slug whose project resolves to the target UUID. Tolerates
	// hand-edited frontmatter that still uses slugs.
	matches := func(refs []string) bool {
		for _, r := range refs {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if r == target.UUID {
				return true
			}
			if p := a.projects.GetBySlug(r); p != nil && p.UUID == target.UUID {
				return true
			}
		}
		return false
	}

	if a.actions != nil {
		for _, x := range a.actions.List() {
			if matches(x.ProjectIDs) {
				c.Actions = append(c.Actions, x)
			}
		}
		c.Counts.Actions = len(c.Actions)
	}

	if a.prds != nil {
		list, err := a.prds.List()
		if err != nil {
			c.Warnings = append(c.Warnings, "prds: "+err.Error())
		} else {
			for _, x := range list {
				if matches(x.Projects) {
					c.PRDs = append(c.PRDs, x)
				}
			}
		}
		c.Counts.PRDs = len(c.PRDs)
	}

	if a.prototypes != nil {
		list, err := a.prototypes.List()
		if err != nil {
			c.Warnings = append(c.Warnings, "prototypes: "+err.Error())
		} else {
			for _, x := range list {
				if matches(x.Projects) {
					c.Prototypes = append(c.Prototypes, x)
				}
			}
		}
		c.Counts.Prototypes = len(c.Prototypes)
	}

	if a.usecases != nil {
		list, err := a.usecases.List()
		if err != nil {
			c.Warnings = append(c.Warnings, "usecases: "+err.Error())
		} else {
			for _, x := range list {
				if matches(x.Projects) {
					c.UseCases = append(c.UseCases, x)
				}
			}
		}
		c.Counts.UseCases = len(c.UseCases)
	}

	if a.shapeup != nil {
		list, err := a.shapeup.ListThreads()
		if err != nil {
			c.Warnings = append(c.Warnings, "threads: "+err.Error())
		} else {
			for _, x := range list {
				if matches(x.Projects) {
					c.Threads = append(c.Threads, x)
				}
			}
		}
		c.Counts.Threads = len(c.Threads)
	}

	if a.documents != nil {
		list, err := a.documents.List()
		if err != nil {
			c.Warnings = append(c.Warnings, "documents: "+err.Error())
		} else {
			for _, x := range list {
				if matches(x.Projects) {
					c.Documents = append(c.Documents, x)
				}
			}
		}
		c.Counts.Documents = len(c.Documents)
	}

	if a.meetings != nil {
		list, err := a.meetings.List()
		if err != nil {
			c.Warnings = append(c.Warnings, "meetings: "+err.Error())
		} else {
			for _, x := range list {
				if matches(x.Projects) {
					c.Meetings = append(c.Meetings, x)
				}
			}
		}
		c.Counts.Meetings = len(c.Meetings)
	}

	return c, true
}
