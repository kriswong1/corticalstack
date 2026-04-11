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

const prdsDir = "prds"

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

// Write persists a PRD to the vault.
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
	p.Path = relPathFor(p)

	note := &vault.Note{Frontmatter: buildFrontmatter(p), Body: p.Body}
	return s.vault.WriteNote(p.Path, note)
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
	if v, ok := note.Frontmatter["version"].(int); ok {
		p.Version = v
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
	p.Body = note.Body
	return p
}
