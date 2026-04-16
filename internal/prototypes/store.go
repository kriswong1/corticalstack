package prototypes

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/stage"
	"github.com/kriswong/corticalstack/internal/vault"
)

const prototypesDir = "prototypes"

// Store manages prototype folders in vault/prototypes/<date>_<slug>/.
type Store struct {
	vault *vault.Vault
}

// New creates a store bound to a vault.
func New(v *vault.Vault) *Store { return &Store{vault: v} }

// EnsureFolder creates vault/prototypes/.
func (s *Store) EnsureFolder() error {
	return os.MkdirAll(filepath.Join(s.vault.Path(), prototypesDir), 0o700)
}

// Write persists a prototype's spec.md + source-links.md in a per-prototype
// subfolder and returns the folder-relative path.
func (s *Store) Write(p *Prototype) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Created.IsZero() {
		p.Created = time.Now()
	}
	if p.Status == "" {
		p.Status = "draft"
	}
	if p.Stage == "" {
		// Default to in_progress for newly written prototypes — the
		// synthesizer just produced a draft, which is past the "need"
		// state but not yet "final".
		p.Stage = stage.StageInProgress
	}

	date := p.Created.Format("2006-01-02")
	slug := vault.Slugify(p.Title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}
	folder := filepath.ToSlash(filepath.Join(prototypesDir, fmt.Sprintf("%s_%s", date, slug)))
	p.FolderPath = folder

	// spec.md with frontmatter + body
	specNote := &vault.Note{
		Frontmatter: buildFrontmatter(p),
		Body:        p.Spec,
	}
	if err := s.vault.WriteNote(filepath.Join(folder, "spec.md"), specNote); err != nil {
		return fmt.Errorf("writing spec: %w", err)
	}

	// prototype.html for raw-HTML formats (interactive-html).
	if strings.TrimSpace(p.HTMLBody) != "" {
		htmlPath := filepath.Join(s.vault.Path(), folder, "prototype.html")
		if err := os.WriteFile(htmlPath, []byte(p.HTMLBody), 0o600); err != nil {
			return fmt.Errorf("writing prototype.html: %w", err)
		}
		p.HasHTML = true
	}

	// source-links.md
	var body strings.Builder
	body.WriteString(fmt.Sprintf("# Source Links — %s\n\n", p.Title))
	for _, ref := range p.SourceRefs {
		body.WriteString(fmt.Sprintf("- [[%s]]\n", strings.TrimSuffix(ref, ".md")))
	}
	linksNote := &vault.Note{
		Frontmatter: map[string]interface{}{
			"id":        p.ID,
			"type":      "prototype-source-links",
			"prototype": p.ID,
			"created":   p.Created.Format(time.RFC3339),
		},
		Body: body.String(),
	}
	if err := s.vault.WriteNote(filepath.Join(folder, "source-links.md"), linksNote); err != nil {
		return fmt.Errorf("writing source-links: %w", err)
	}

	return nil
}

// List returns every prototype in the vault, newest first.
func (s *Store) List() ([]*Prototype, error) {
	dir := filepath.Join(s.vault.Path(), prototypesDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []*Prototype
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(prototypesDir, e.Name(), "spec.md"))
		if !s.vault.Exists(relPath) {
			continue
		}
		note, err := s.vault.ReadNote(relPath)
		if err != nil {
			continue
		}
		p := fromNote(note)
		p.FolderPath = filepath.ToSlash(filepath.Join(prototypesDir, e.Name()))
		htmlPath := filepath.Join(dir, e.Name(), "prototype.html")
		if _, err := os.Stat(htmlPath); err == nil {
			p.HasHTML = true
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Vault exposes the bound vault for callers that need to read sources.
func (s *Store) Vault() *vault.Vault { return s.vault }

// SetStage rewrites the `stage` (and `updated`) frontmatter keys on
// the prototype's spec.md. The body, source links, and prototype.html
// are left untouched. Returns an error if the id is unknown, the
// stage is invalid for the prototype entity, or the disk write fails.
func (s *Store) SetStage(id string, target stage.Stage) error {
	if !stage.Validate(stage.EntityPrototype, string(target)) {
		return fmt.Errorf("invalid prototype stage: %q", target)
	}
	list, err := s.List()
	if err != nil {
		return err
	}
	var found *Prototype
	for _, p := range list {
		if p.ID == id {
			found = p
			break
		}
	}
	if found == nil {
		return fmt.Errorf("prototype not found: %s", id)
	}
	specRel := filepath.ToSlash(filepath.Join(found.FolderPath, "spec.md"))
	note, err := s.vault.ReadNote(specRel)
	if err != nil {
		return fmt.Errorf("reading %s: %w", specRel, err)
	}
	if note.Frontmatter == nil {
		note.Frontmatter = map[string]interface{}{}
	}
	note.Frontmatter["stage"] = string(target)
	note.Frontmatter["updated"] = time.Now().Format(time.RFC3339)
	if err := s.vault.WriteNote(specRel, note); err != nil {
		return fmt.Errorf("writing %s: %w", specRel, err)
	}
	return nil
}

// ReadHTML returns the prototype.html contents for a prototype by ID.
// Returns os.ErrNotExist if the prototype exists but has no HTML file.
func (s *Store) ReadHTML(id string) ([]byte, *Prototype, error) {
	list, err := s.List()
	if err != nil {
		return nil, nil, err
	}
	for _, p := range list {
		if p.ID != id {
			continue
		}
		htmlPath := filepath.Join(s.vault.Path(), p.FolderPath, "prototype.html")
		body, err := os.ReadFile(htmlPath)
		if err != nil {
			return nil, p, err
		}
		return body, p, nil
	}
	return nil, nil, fmt.Errorf("prototype not found: %s", id)
}

func buildFrontmatter(p *Prototype) map[string]interface{} {
	fm := map[string]interface{}{
		"id":      p.ID,
		"type":    "prototype",
		"format":  p.Format,
		"title":   p.Title,
		"status":  p.Status,
		"stage":   string(p.Stage),
		"created": p.Created.Format(time.RFC3339),
	}
	if !p.Updated.IsZero() {
		fm["updated"] = p.Updated.Format(time.RFC3339)
	}
	if len(p.SourceRefs) > 0 {
		fm["source_refs"] = p.SourceRefs
	}
	if p.SourceThread != "" {
		fm["source_thread"] = p.SourceThread
	}
	if len(p.Projects) > 0 {
		fm["projects"] = p.Projects
	}
	fm["tags"] = []string{"cortical", "prototype", p.Format}
	return fm
}

func fromNote(note *vault.Note) *Prototype {
	p := &Prototype{}
	if id, ok := note.Frontmatter["id"].(string); ok {
		p.ID = id
	}
	if title, ok := note.Frontmatter["title"].(string); ok {
		p.Title = title
	}
	if format, ok := note.Frontmatter["format"].(string); ok {
		p.Format = format
	}
	if status, ok := note.Frontmatter["status"].(string); ok {
		p.Status = status
	}
	// Stage is the new dashboard-facing field. Read it directly when
	// present; otherwise derive it from the legacy `status` field via
	// stage.Normalize (draft → in_progress, exported → final). This
	// keeps every existing on-disk prototype landing in a sensible
	// dashboard bucket without a migration script.
	if rawStage, ok := note.Frontmatter["stage"].(string); ok {
		p.Stage = stage.Normalize(stage.EntityPrototype, rawStage)
	} else {
		p.Stage = stage.Normalize(stage.EntityPrototype, p.Status)
	}
	if thread, ok := note.Frontmatter["source_thread"].(string); ok {
		p.SourceThread = thread
	}
	if refs, ok := note.Frontmatter["source_refs"].([]interface{}); ok {
		for _, r := range refs {
			if s, ok := r.(string); ok {
				p.SourceRefs = append(p.SourceRefs, s)
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
	if created, ok := note.Frontmatter["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			p.Created = t
		}
	}
	if updated, ok := note.Frontmatter["updated"].(string); ok {
		if t, err := time.Parse(time.RFC3339, updated); err == nil {
			p.Updated = t
		}
	}
	p.Spec = note.Body
	return p
}
