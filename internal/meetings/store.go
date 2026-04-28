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

// stagePlural maps a stage key to its on-disk subfolder.
func stagePlural(st stage.Stage) string {
	switch st {
	case stage.StageAudio:
		return "audio"
	case stage.StageTranscript:
		return "transcripts"
	case stage.StageNote:
		return "notes"
	default:
		return string(st)
	}
}

// folderStage maps an on-disk folder name back to a Stage. Includes
// the legacy "summaries" folder so older notes still surface in List().
func folderStage(folder string) (stage.Stage, bool) {
	switch folder {
	case "audio":
		return stage.StageAudio, true
	case "transcripts":
		return stage.StageTranscript, true
	case "notes":
		return stage.StageNote, true
	case "summaries":
		return stage.StageNote, true
	}
	return "", false
}

// scanFolders is the canonical list of subfolders the store reads
// from. Includes the legacy "summaries" folder for backward compat.
func scanFolders() []string {
	return []string{"audio", "transcripts", "notes", "summaries"}
}

// audioExts are the file extensions recognised as audio-stage
// meetings when found under meetings/audio/. Mirrors the set the
// Deepgram transformer accepts in CanHandle.
var audioExts = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".m4a":  true,
	".ogg":  true,
	".flac": true,
	".webm": true,
}

// List returns every meeting record in the vault, newest first. A
// missing meetings folder returns an empty slice with no error — the
// dashboard renders empty pipelines gracefully.
//
// Audio files in meetings/audio/ surface as Audio-stage meetings only
// when no transcript references them via `source_audio` frontmatter.
// Once transcribed, the meeting moves to the transcript record and
// the underlying audio file no longer counts as a separate entry.
func (s *Store) List() ([]*Meeting, error) {
	root := filepath.Join(s.vault.Path(), meetingsDir)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var out []*Meeting
	claimedAudio := map[string]bool{}

	for _, folder := range scanFolders() {
		dir := filepath.Join(root, folder)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		dirStage, ok := folderStage(folder)
		if !ok {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			relPath := filepath.ToSlash(filepath.Join(meetingsDir, folder, e.Name()))

			if dirStage == stage.StageAudio {
				if !audioExts[strings.ToLower(filepath.Ext(e.Name()))] {
					continue
				}
				m, err := s.readAudio(relPath)
				if err != nil {
					slog.Warn("meetings: skipping audio", "path", relPath, "error", err)
					continue
				}
				out = append(out, m)
				continue
			}

			if !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			m, err := s.readMeeting(relPath, dirStage)
			if err != nil {
				slog.Warn("meetings: skipping note", "path", relPath, "error", err)
				continue
			}
			if m.SourceAudio != "" {
				claimedAudio[m.SourceAudio] = true
			}
			out = append(out, m)
		}
	}

	if len(claimedAudio) > 0 {
		out = filterClaimedAudio(out, claimedAudio)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Created.After(out[j].Created)
	})
	return out, nil
}

// filterClaimedAudio drops audio-stage entries whose path is referenced
// by some transcript or note's source_audio frontmatter — that meeting
// has progressed past Audio and shouldn't double-count.
func filterClaimedAudio(in []*Meeting, claimed map[string]bool) []*Meeting {
	out := in[:0]
	for _, m := range in {
		if m.Stage == stage.StageAudio && claimed[m.Path] {
			continue
		}
		out = append(out, m)
	}
	return out
}

// readAudio builds a Meeting record from a raw audio file under
// vault/meetings/audio/. ID and title come from the filename stem
// since audio files have no frontmatter to read.
func (s *Store) readAudio(relPath string) (*Meeting, error) {
	abs := filepath.Join(s.vault.Path(), relPath)
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	stem := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	return &Meeting{
		ID:      stem,
		Title:   stem,
		Stage:   stage.StageAudio,
		Path:    relPath,
		Created: info.ModTime(),
		Updated: info.ModTime(),
	}, nil
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
	if sa, ok := fm["source_audio"].(string); ok {
		m.SourceAudio = filepath.ToSlash(sa)
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
