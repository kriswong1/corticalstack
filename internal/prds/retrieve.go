package prds

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kriswong/corticalstack/internal/vault"
)

// Bucket is one context category used during PRD synthesis. Each bucket
// has a default tag set that the retriever matches against every note's
// frontmatter tags.
type Bucket struct {
	Name        string
	DefaultTags []string
}

// DefaultBuckets returns the three canonical context buckets spec'd in
// docs/05-prd-synthesis.md.
func DefaultBuckets() []Bucket {
	return []Bucket{
		{
			Name:        "engineering",
			DefaultTags: []string{"architecture", "codebase-summary", "constraint", "engineering-decision"},
		},
		{
			Name:        "design",
			DefaultTags: []string{"design-system", "design-pattern", "design-language"},
		},
		{
			Name:        "product",
			DefaultTags: []string{"competitive-analysis", "user-research", "business-constraint"},
		},
	}
}

// RetrievedNote is one note found by the retriever with its path + body.
type RetrievedNote struct {
	Path    string
	Title   string
	Body    string
	Tags    []string
	Bucket  string
}

// Retriever globs the vault for notes whose frontmatter tags match a
// bucket's default tag set. Everything is deterministic — no Claude calls.
type Retriever struct {
	vault           *vault.Vault
	perBucketLimit  int
}

// NewRetriever creates a retriever with the default per-bucket limit (5).
func NewRetriever(v *vault.Vault) *Retriever {
	return &Retriever{vault: v, perBucketLimit: 5}
}

// WithLimit sets a custom per-bucket cap.
func (r *Retriever) WithLimit(n int) *Retriever {
	r.perBucketLimit = n
	return r
}

// Retrieve runs over every markdown file in the vault and returns the
// top-N per bucket, ranked by (project-match, recency).
func (r *Retriever) Retrieve(projectIDs, extraTags, extraPaths []string) ([]RetrievedNote, error) {
	allNotes, err := r.walkAllNotes()
	if err != nil {
		return nil, err
	}

	// Include explicit extraPaths regardless of tags.
	explicit := make(map[string]bool)
	for _, p := range extraPaths {
		explicit[p] = true
	}

	buckets := DefaultBuckets()

	// per-bucket candidate pools
	pools := make(map[string][]RetrievedNote)
	projectSet := toSet(projectIDs)
	for _, n := range allNotes {
		tagSet := toSet(n.Tags)
		inProject := anyMatch(tagSet, projectSet) || noteInProjects(n, projectIDs)

		for _, b := range buckets {
			bucketTags := append([]string{}, b.DefaultTags...)
			if anyMatchAny(tagSet, bucketTags) || anyMatchAny(tagSet, extraTags) {
				copy := n
				copy.Bucket = b.Name
				if inProject {
					// Project match boosts sort priority.
					copy.Tags = append(copy.Tags, "_project_match")
				}
				pools[b.Name] = append(pools[b.Name], copy)
			}
		}
	}

	// Also include any explicit paths (fetch + tag as "extra").
	var extras []RetrievedNote
	for _, p := range extraPaths {
		body, err := r.vault.ReadFile(p)
		if err != nil {
			continue
		}
		extras = append(extras, RetrievedNote{
			Path:   p,
			Title:  filepath.Base(p),
			Body:   body,
			Bucket: "extra",
		})
	}

	var out []RetrievedNote
	for _, b := range buckets {
		pool := pools[b.Name]
		sort.SliceStable(pool, func(i, j int) bool {
			pi := containsString(pool[i].Tags, "_project_match")
			pj := containsString(pool[j].Tags, "_project_match")
			if pi != pj {
				return pi
			}
			// Newer path name (by date prefix) wins.
			return pool[i].Path > pool[j].Path
		})
		if len(pool) > r.perBucketLimit {
			pool = pool[:r.perBucketLimit]
		}
		for i := range pool {
			pool[i].Tags = removeString(pool[i].Tags, "_project_match")
		}
		out = append(out, pool...)
	}
	out = append(out, extras...)
	return out, nil
}

// walkAllNotes returns every .md file under the vault with its parsed
// frontmatter tags and body.
func (r *Retriever) walkAllNotes() ([]RetrievedNote, error) {
	root := r.vault.Path()
	var notes []RetrievedNote

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		// Skip hidden directories like .cortical.
		if strings.Contains(path, string(filepath.Separator)+".") {
			return nil
		}
		relPath, _ := filepath.Rel(root, path)
		relPath = filepath.ToSlash(relPath)

		note, err := r.vault.ReadNote(relPath)
		if err != nil {
			return nil
		}
		n := RetrievedNote{
			Path: relPath,
			Body: note.Body,
		}
		if title, ok := note.Frontmatter["title"].(string); ok {
			n.Title = title
		}
		if n.Title == "" {
			n.Title = strings.TrimSuffix(filepath.Base(relPath), ".md")
		}
		if tags, ok := note.Frontmatter["tags"].([]interface{}); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					n.Tags = append(n.Tags, s)
				}
			}
		}
		notes = append(notes, n)
		return nil
	})
	return notes, err
}

func noteInProjects(n RetrievedNote, projects []string) bool {
	for _, p := range projects {
		if containsString(n.Tags, p) {
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, i := range items {
		s[strings.ToLower(strings.TrimSpace(i))] = true
	}
	return s
}

func anyMatch(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func anyMatchAny(set map[string]bool, candidates []string) bool {
	for _, c := range candidates {
		if set[strings.ToLower(strings.TrimSpace(c))] {
			return true
		}
	}
	return false
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func removeString(list []string, target string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}
