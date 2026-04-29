package prds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/vault"
)

const (
	prdsDir     = "prds"
	versionsDir = "_versions"
)

// Store manages PRD files at vault/prds/<date>_<slug>.md.
type Store struct {
	vault *vault.Vault
}

// New creates a store bound to a vault.
func New(v *vault.Vault) *Store { return &Store{vault: v} }

// EnsureFolder creates vault/prds/.
func (s *Store) EnsureFolder() error {
	return os.MkdirAll(filepath.Join(s.vault.Path(), prdsDir), 0o700)
}

// Write persists a PRD to the vault. If p.Path is already set (refine
// flow), it's preserved — otherwise a fresh date_slug.md path is
// computed. Preserving Path on refine keeps the URL stable even when
// Claude proposes a different title for the new version.
func (s *Store) Write(p *PRD) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Created.IsZero() {
		p.Created = time.Now()
	}
	if p.Version == 0 {
		p.Version = 1
	}
	if p.Status == "" {
		p.Status = StatusDraft
	}
	if p.Path == "" {
		p.Path = relPathFor(p)
	}

	note := &vault.Note{Frontmatter: buildFrontmatter(p), Body: p.Body}
	return s.vault.WriteNote(p.Path, note)
}

// ArchiveCurrent copies the current live PRD file into
// `prds/_versions/<id>/v{n}.md`, along with an optional hints.txt
// recording the refinement prompt. Returns nil if the PRD has no
// live file yet (fresh draft). Called by the refine handler AFTER
// synthesis succeeds — on failure the live file stays untouched and
// no ghost archive is produced (mirrors the prototype fix from #17).
func (s *Store) ArchiveCurrent(p *PRD, hints string) error {
	if p.Path == "" || p.ID == "" {
		return fmt.Errorf("prd has no path or id")
	}
	srcFull := filepath.Join(s.vault.Path(), filepath.FromSlash(p.Path))
	data, err := os.ReadFile(srcFull)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading current prd: %w", err)
	}
	verFolder := filepath.Join(s.vault.Path(), prdsDir, versionsDir, p.ID)
	if err := os.MkdirAll(verFolder, 0o700); err != nil {
		return fmt.Errorf("creating version folder: %w", err)
	}
	dst := filepath.Join(verFolder, fmt.Sprintf("v%d.md", p.Version))
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("writing archived prd: %w", err)
	}
	if strings.TrimSpace(hints) != "" {
		hintsPath := filepath.Join(verFolder, fmt.Sprintf("v%d.hints.txt", p.Version))
		if err := os.WriteFile(hintsPath, []byte(hints), 0o600); err != nil {
			return fmt.Errorf("writing hints: %w", err)
		}
	}
	return nil
}

// ReadVersion returns the archived body bytes for a specific version
// of a PRD. Used by the PRD detail page's version switcher so the
// user can inspect an earlier draft without affecting the live file.
// Returns os.ErrNotExist when the version folder exists but the
// requested version file is missing.
func (s *Store) ReadVersion(id string, version int) (string, error) {
	path := filepath.Join(s.vault.Path(), prdsDir, versionsDir, id, fmt.Sprintf("v%d.md", version))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListVersions returns every archived version for a PRD, oldest first.
// An empty list means the PRD is still on v1. Does not include the
// live version — callers get that from the PRD itself.
func (s *Store) ListVersions(id string) ([]VersionInfo, error) {
	verRoot := filepath.Join(s.vault.Path(), prdsDir, versionsDir, id)
	entries, err := os.ReadDir(verRoot)
	if os.IsNotExist(err) {
		return []VersionInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []VersionInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "v") || !strings.HasSuffix(name, ".md") {
			continue
		}
		numPart := strings.TrimSuffix(strings.TrimPrefix(name, "v"), ".md")
		var n int
		if _, scanErr := fmt.Sscanf(numPart, "%d", &n); scanErr != nil || n <= 0 {
			continue
		}
		info := VersionInfo{Version: n}
		if stat, err := os.Stat(filepath.Join(verRoot, name)); err == nil {
			info.Created = stat.ModTime()
		}
		hintsPath := filepath.Join(verRoot, fmt.Sprintf("v%d.hints.txt", n))
		if data, err := os.ReadFile(hintsPath); err == nil {
			info.Hints = strings.TrimSpace(string(data))
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Get returns a single PRD by relative path.
func (s *Store) Get(relPath string) (*PRD, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}
	p := fromNote(note)
	p.Path = relPath
	return p, nil
}

// List returns every PRD, newest first.
func (s *Store) List() ([]*PRD, error) {
	dir := filepath.Join(s.vault.Path(), prdsDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*PRD
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(prdsDir, e.Name()))
		p, err := s.Get(relPath)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Vault exposes the bound vault.
func (s *Store) Vault() *vault.Vault { return s.vault }

// validStatus lists the statuses SetStatus will accept.
var validStatus = map[Status]bool{
	StatusDraft:    true,
	StatusReview:   true,
	StatusApproved: true,
	StatusShipped:  true,
	StatusArchived: true,
}

// SetStatus rewrites the frontmatter `status` on the PRD identified by
// id. The PRD's path (and body) is preserved — only the metadata
// field changes. Returns an error if the status is unknown or the PRD
// cannot be located by id.
func (s *Store) SetStatus(id string, status Status) (*PRD, error) {
	if id == "" {
		return nil, fmt.Errorf("id required")
	}
	if !validStatus[status] {
		return nil, fmt.Errorf("invalid PRD status: %q", status)
	}
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	var found *PRD
	for _, p := range list {
		if p.ID == id {
			found = p
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("prd not found: %s", id)
	}
	note, err := s.vault.ReadNote(found.Path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", found.Path, err)
	}
	if note.Frontmatter == nil {
		note.Frontmatter = map[string]interface{}{}
	}
	note.Frontmatter["status"] = string(status)
	if err := s.vault.WriteNote(found.Path, note); err != nil {
		return nil, fmt.Errorf("writing %s: %w", found.Path, err)
	}
	found.Status = status
	return found, nil
}

func relPathFor(p *PRD) string {
	date := p.Created.Format("2006-01-02")
	slug := vault.Slugify(p.Title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}
	return filepath.ToSlash(filepath.Join(prdsDir, fmt.Sprintf("%s_%s.md", date, slug)))
}

func buildFrontmatter(p *PRD) map[string]interface{} {
	fm := map[string]interface{}{
		"id":                   p.ID,
		"type":                 "prd",
		"version":              p.Version,
		"status":               string(p.Status),
		"title":                p.Title,
		"source_pitch":         p.SourcePitch,
		"open_questions_count": p.OpenQuestionsCount,
		"created":              p.Created.Format(time.RFC3339),
	}
	if p.SourceThread != "" {
		fm["source_thread"] = p.SourceThread
	}
	if len(p.ContextRefs) > 0 {
		fm["context_refs"] = p.ContextRefs
	}
	if len(p.Projects) > 0 {
		fm["projects"] = p.Projects
	}
	if p.LinearDocumentID != "" {
		fm["linear_document_id"] = p.LinearDocumentID
	}
	fm["tags"] = []string{"cortical", "prd"}
	return fm
}

func fromNote(note *vault.Note) *PRD {
	p := &PRD{Version: 1, Status: StatusDraft}
	if id, ok := note.Frontmatter["id"].(string); ok {
		p.ID = id
	}
	if title, ok := note.Frontmatter["title"].(string); ok {
		p.Title = title
	}
	// Version can be int / int64 / float64 depending on how the YAML
	// parser interpreted the frontmatter value.
	switch v := note.Frontmatter["version"].(type) {
	case int:
		p.Version = v
	case int64:
		p.Version = int(v)
	case float64:
		p.Version = int(v)
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	if status, ok := note.Frontmatter["status"].(string); ok {
		p.Status = Status(status)
	}
	if pitch, ok := note.Frontmatter["source_pitch"].(string); ok {
		p.SourcePitch = pitch
	}
	if refs, ok := note.Frontmatter["context_refs"].([]interface{}); ok {
		for _, r := range refs {
			if s, ok := r.(string); ok {
				p.ContextRefs = append(p.ContextRefs, s)
			}
		}
	}
	if projects, ok := note.Frontmatter["projects"].([]interface{}); ok {
		for _, pr := range projects {
			if s, ok := pr.(string); ok {
				p.Projects = append(p.Projects, s)
			}
		}
	}
	if oq, ok := note.Frontmatter["open_questions_count"].(int); ok {
		p.OpenQuestionsCount = oq
	}
	if created, ok := note.Frontmatter["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			p.Created = t
		}
	}
	if lid, ok := note.Frontmatter["linear_document_id"].(string); ok {
		p.LinearDocumentID = lid
	}
	p.Body = note.Body
	return p
}
