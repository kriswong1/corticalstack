package usecases

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

const useCasesDir = "usecases"

// Store reads and writes UseCase markdown files in vault/usecases/.
type Store struct {
	vault *vault.Vault
}

// New creates a store bound to a vault.
func New(v *vault.Vault) *Store {
	return &Store{vault: v}
}

// EnsureFolder creates vault/usecases/ if it doesn't exist.
func (s *Store) EnsureFolder() error {
	return os.MkdirAll(filepath.Join(s.vault.Path(), useCasesDir), 0o700)
}

// Write persists a UseCase to disk and fills in its Path and created time.
func (s *Store) Write(u *UseCase) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	if u.Created.IsZero() {
		u.Created = time.Now()
	}
	u.Path = relPathFor(u)

	note := &vault.Note{Frontmatter: buildFrontmatter(u), Body: buildBody(u)}
	return s.vault.WriteNote(u.Path, note)
}

// Get returns a single UseCase by its relative path.
func (s *Store) Get(relPath string) (*UseCase, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}
	u := fromNote(note)
	u.Path = relPath
	return u, nil
}

// List returns every UseCase in the vault, newest first.
func (s *Store) List() ([]*UseCase, error) {
	dir := filepath.Join(s.vault.Path(), useCasesDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []*UseCase
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(useCasesDir, e.Name()))
		u, err := s.Get(relPath)
		if err != nil {
			continue
		}
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Vault returns the bound vault for handlers that need to read source
// documents during generation.
func (s *Store) Vault() *vault.Vault { return s.vault }

func relPathFor(u *UseCase) string {
	date := u.Created.Format("2006-01-02")
	slug := vault.Slugify(u.Title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}
	return filepath.ToSlash(filepath.Join(useCasesDir, fmt.Sprintf("%s_%s.md", date, slug)))
}

func buildFrontmatter(u *UseCase) map[string]interface{} {
	fm := map[string]interface{}{
		"id":         u.ID,
		"type":       "usecase",
		"title":      u.Title,
		"actors":     u.Actors,
		"main_flow":  u.MainFlow,
		"created":    u.Created.Format(time.RFC3339),
	}
	if len(u.SecondaryActors) > 0 {
		fm["secondary_actors"] = u.SecondaryActors
	}
	if len(u.Preconditions) > 0 {
		fm["preconditions"] = u.Preconditions
	}
	if len(u.AlternativeFlows) > 0 {
		flows := make([]map[string]interface{}, 0, len(u.AlternativeFlows))
		for _, f := range u.AlternativeFlows {
			flows = append(flows, map[string]interface{}{
				"name":    f.Name,
				"at_step": f.AtStep,
				"flow":    f.Flow,
			})
		}
		fm["alternative_flows"] = flows
	}
	if len(u.Postconditions) > 0 {
		fm["postconditions"] = u.Postconditions
	}
	if len(u.BusinessRules) > 0 {
		fm["business_rules"] = u.BusinessRules
	}
	if len(u.NonFunctional) > 0 {
		fm["non_functional"] = u.NonFunctional
	}
	if len(u.Sources) > 0 {
		sources := make([]map[string]string, 0, len(u.Sources))
		for _, src := range u.Sources {
			sources = append(sources, map[string]string{"type": src.Type, "path": src.Path})
		}
		fm["source"] = sources
	}
	if len(u.Projects) > 0 {
		fm["projects"] = u.Projects
	}

	tags := []string{"cortical", "usecase"}
	tags = append(tags, u.Tags...)
	fm["tags"] = tags

	return fm
}

func buildBody(u *UseCase) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", u.Title))

	if len(u.Actors) > 0 {
		b.WriteString(fmt.Sprintf("**Actors:** %s\n\n", strings.Join(u.Actors, ", ")))
	}
	if len(u.SecondaryActors) > 0 {
		b.WriteString(fmt.Sprintf("**Secondary actors:** %s\n\n", strings.Join(u.SecondaryActors, ", ")))
	}

	writeSection(&b, "Preconditions", u.Preconditions)

	if len(u.MainFlow) > 0 {
		b.WriteString("## Main Flow\n\n")
		for i, step := range u.MainFlow {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
		b.WriteString("\n")
	}

	for _, alt := range u.AlternativeFlows {
		b.WriteString(fmt.Sprintf("## Alternative: %s (at step %d)\n\n", alt.Name, alt.AtStep))
		for i, step := range alt.Flow {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
		b.WriteString("\n")
	}

	writeSection(&b, "Postconditions", u.Postconditions)
	writeSection(&b, "Business Rules", u.BusinessRules)
	writeSection(&b, "Non-functional Requirements", u.NonFunctional)

	if len(u.Sources) > 0 {
		b.WriteString("## Sources\n\n")
		for _, src := range u.Sources {
			if src.Path != "" {
				b.WriteString(fmt.Sprintf("- [%s] [[%s]]\n", src.Type, strings.TrimSuffix(src.Path, ".md")))
			} else {
				b.WriteString(fmt.Sprintf("- %s\n", src.Type))
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

func writeSection(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf("## %s\n\n", title))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// fromNote reconstructs a UseCase from a parsed markdown note. We only
// pull fields needed for listing/previewing; full round-trip parsing of
// alternative_flows is skipped because we never need to regenerate a note
// from its parsed form (it's always written fresh from a generator).
func fromNote(note *vault.Note) *UseCase {
	u := &UseCase{}
	if id, ok := note.Frontmatter["id"].(string); ok {
		u.ID = id
	}
	if title, ok := note.Frontmatter["title"].(string); ok {
		u.Title = title
	}
	if actors, ok := note.Frontmatter["actors"].([]interface{}); ok {
		for _, a := range actors {
			if s, ok := a.(string); ok {
				u.Actors = append(u.Actors, s)
			}
		}
	}
	if projects, ok := note.Frontmatter["projects"].([]interface{}); ok {
		for _, p := range projects {
			if s, ok := p.(string); ok {
				u.Projects = append(u.Projects, s)
			}
		}
	}
	if created, ok := note.Frontmatter["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			u.Created = t
		}
	}
	return u
}
