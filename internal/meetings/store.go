package meetings

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/stage"
	"github.com/kriswong/corticalstack/internal/vault"
)

const meetingsDir = "meetings"

// Store is a read/write scanner over the meetings folder in the vault.
// Mirrors the shapeup/prds pattern: the vault is the source of truth.
// The only mutation this store performs is SetStage, used by the
// unified dashboard to advance a meeting through the per-card stage
// flow without leaving the page.
type Store struct {
	vault *vault.Vault
}

// New returns a meetings store bound to a vault.
func New(v *vault.Vault) *Store {
	return &Store{vault: v}
}

// EnsureFolder creates vault/meetings/{transcripts,audio,notes}/ if
// missing. Called from main on startup so the dashboard always has a
// stable place to look even on a fresh vault. Failure is logged, not
// fatal — the vault may be on a read-only mount the user mounts later.
func (s *Store) EnsureFolder() error {
	for _, st := range AllStages() {
		dir := filepath.Join(s.vault.Path(), meetingsDir, stagePlural(st))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

// stagePlural maps a stage key to its on-disk subfolder. We pluralize
// for filesystem readability ("transcripts" reads better than
// "transcript" in `ls`) while keeping the canonical Stage value
// singular for API responses. The `audio` stage is intentionally NOT
// pluralized — "audios" is awkward and "audio" reads as a mass noun.
func stagePlural(st stage.Stage) string {
	switch st {
	case stage.StageTranscript:
		return "transcripts"
	case stage.StageAudio:
		return "audio"
	case stage.StageNote:
		return "notes"
	default:
		return string(st)
	}
}

// legacyFolderStage maps an on-disk folder name back to a Stage,
// including the historical "summaries" folder which is now treated as
// "notes" — the rename happened with the unified-dashboard refactor.
// Existing notes in vault/meetings/summaries/ keep classifying as
// StageNote without a migration script.
func legacyFolderStage(folder string) (stage.Stage, bool) {
	switch folder {
	case "transcripts":
		return stage.StageTranscript, true
	case "audio":
		return stage.StageAudio, true
	case "notes":
		return stage.StageNote, true
	case "summaries":
		// Legacy: pre-refactor folder name. Treat as Notes.
		return stage.StageNote, true
	}
	return "", false
}

// scanFolders is the canonical list of subfolders the store reads
// from. New canonical folders plus the legacy "summaries" folder so
// existing on-disk notes still surface in List().
func scanFolders() []string {
	return []string{"transcripts", "audio", "notes", "summaries"}
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
	for _, folder := range scanFolders() {
		dir := filepath.Join(root, folder)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		dirStage, ok := legacyFolderStage(folder)
		if !ok {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			relPath := filepath.ToSlash(filepath.Join(meetingsDir, folder, e.Name()))
			m, err := s.readMeeting(relPath, dirStage)
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

// SetStage rewrites the `stage` (and `updated`) frontmatter keys on
// the meeting note. The body is preserved as-is and the file stays
// in its current folder — moving the file across folders would
// invalidate any wikilinks pointing at it. The dashboard reads stage
// from frontmatter first, folder second, so updating frontmatter is
// enough to reclassify.
func (s *Store) SetStage(id string, target stage.Stage) error {
	if !stage.Validate(stage.EntityMeeting, string(target)) {
		return fmt.Errorf("invalid meeting stage: %q", target)
	}
	list, err := s.List()
	if err != nil {
		return err
	}
	var found *Meeting
	for _, m := range list {
		if m.ID == id {
			found = m
			break
		}
	}
	if found == nil {
		return fmt.Errorf("meeting not found: %s", id)
	}
	note, err := s.vault.ReadNote(found.Path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", found.Path, err)
	}
	if note.Frontmatter == nil {
		note.Frontmatter = map[string]interface{}{}
	}
	note.Frontmatter["stage"] = string(target)
	note.Frontmatter["updated"] = time.Now().Format(time.RFC3339)
	if err := s.vault.WriteNote(found.Path, note); err != nil {
		return fmt.Errorf("writing %s: %w", found.Path, err)
	}
	return nil
}

// readMeeting parses a single markdown file's frontmatter into a
// Meeting. Stage is taken from the frontmatter if present, else from
// the directory the file lives in (the filesystem layout is the
// fallback source of truth, so a freshly-dropped transcript without
// frontmatter still classifies correctly).
func (s *Store) readMeeting(relPath string, dirStage stage.Stage) (*Meeting, error) {
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
	if rawStage, ok := fm["stage"].(string); ok {
		// Normalize handles legacy "summary" → StageNote so on-disk
		// notes that predate the rename still classify as Notes.
		m.Stage = stage.Normalize(stage.EntityMeeting, rawStage)
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
