package meetings

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

const meetingsDir = "meetings"

// Store is a read-only scanner over the meetings folder in the vault.
// Mirrors the shapeup/prds pattern: vault is the source of truth, this
// type does no writes of its own.
type Store struct {
	vault *vault.Vault
}

// New returns a meetings store bound to a vault.
func New(v *vault.Vault) *Store {
	return &Store{vault: v}
}

// EnsureFolder creates vault/meetings/{transcripts,summaries}/ if missing.
// Called from main on startup so the dashboard always has a stable
// place to look even on a fresh vault. Failure is logged, not fatal —
// the vault may be on a read-only mount the user mounts later.
func (s *Store) EnsureFolder() error {
	for _, stage := range AllStages() {
		dir := filepath.Join(s.vault.Path(), meetingsDir, stagePlural(stage))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

// stagePlural maps a stage key to its on-disk subfolder. We pluralize
// for filesystem readability ("transcripts" reads better than
// "transcript" in `ls`) while keeping the canonical Stage value
// singular for API responses.
func stagePlural(stage Stage) string {
	switch stage {
	case StageTranscript:
		return "transcripts"
	case StageSummary:
		return "summaries"
	default:
		return string(stage)
	}
}

// List returns every meeting note in the vault, newest first. A
// missing meetings folder returns an empty slice with no error — the
// dashboard renders empty pipelines gracefully.
func (s *Store) List() ([]*Meeting, error) {
	root := filepath.Join(s.vault.Path(), meetingsDir)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var out []*Meeting
	for _, stage := range AllStages() {
		dir := filepath.Join(root, stagePlural(stage))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			relPath := filepath.ToSlash(filepath.Join(meetingsDir, stagePlural(stage), e.Name()))
			m, err := s.readMeeting(relPath, stage)
			if err != nil {
				slog.Warn("meetings: skipping note", "path", relPath, "error", err)
				continue
			}
			out = append(out, m)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Created.After(out[j].Created)
	})
	return out, nil
}

// readMeeting parses a single markdown file's frontmatter into a
// Meeting. Stage is taken from the frontmatter if present, else from
// the directory the file lives in (the filesystem layout is the
// fallback source of truth, so a freshly-dropped transcript without
// frontmatter still classifies correctly).
func (s *Store) readMeeting(relPath string, dirStage Stage) (*Meeting, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}

	m := &Meeting{
		Path:  relPath,
		Stage: dirStage,
	}

	fm := note.Frontmatter
	if id, ok := fm["id"].(string); ok {
		m.ID = id
	}
	if m.ID == "" {
		// Fall back to the filename stem so dashboard items always
		// have a stable key even on hand-dropped notes without
		// frontmatter.
		m.ID = strings.TrimSuffix(filepath.Base(relPath), ".md")
	}
	if title, ok := fm["title"].(string); ok && title != "" {
		m.Title = title
	} else {
		m.Title = strings.TrimSuffix(filepath.Base(relPath), ".md")
	}
	if stage, ok := fm["stage"].(string); ok && IsValidStage(stage) {
		m.Stage = Stage(stage)
	}
	if src, ok := fm["source_id"].(string); ok {
		m.SourceID = src
	}
	if sp, ok := fm["source_path"].(string); ok {
		m.SourcePath = sp
	}
	if projects, ok := fm["projects"].([]interface{}); ok {
		for _, p := range projects {
			if str, ok := p.(string); ok {
				m.Projects = append(m.Projects, str)
			}
		}
	}
	if created, ok := fm["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			m.Created = t
		}
	}
	if updated, ok := fm["updated"].(string); ok {
		if t, err := time.Parse(time.RFC3339, updated); err == nil {
			m.Updated = t
		}
	}
	if m.Created.IsZero() {
		// Fall back to file mtime so timeline ordering still works
		// for hand-dropped notes that lack a `created` frontmatter key.
		abs := filepath.Join(s.vault.Path(), relPath)
		if info, err := os.Stat(abs); err == nil {
			m.Created = info.ModTime()
		}
	}

	return m, nil
}
