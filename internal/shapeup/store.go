package shapeup

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/vault"
)

const productDir = "product"

// Store reads and writes ShapeUp artifacts inside the vault. The vault is
// the source of truth; this store is a thin glob-based reader.
type Store struct {
	vault *vault.Vault
}

// New creates a store bound to a vault.
func New(v *vault.Vault) *Store {
	return &Store{vault: v}
}

// EnsureFolders creates vault/product/{raw,frame,shape,breadboard,pitch}.
func (s *Store) EnsureFolders() error {
	for _, stage := range AllStages() {
		path := filepath.Join(s.vault.Path(), productDir, string(stage))
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", path, err)
		}
	}
	return nil
}

// NewThreadID returns a fresh thread UUID.
func NewThreadID() string { return uuid.NewString() }

// CreateRawIdea writes a new raw idea note. If req.ThreadID is empty a
// fresh thread UUID is allocated (the default — each idea starts its
// own thread). If req.ThreadID is non-empty the new artifact is
// attached to that thread, letting callers append refined raw ideas to
// an existing thread without starting a new arc. See LO-04.
func (s *Store) CreateRawIdea(req CreateIdeaRequest) (*Artifact, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, fmt.Errorf("title required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("content required")
	}

	threadID := strings.TrimSpace(req.ThreadID)
	if threadID == "" {
		threadID = NewThreadID()
	}

	now := time.Now()
	artifact := &Artifact{
		ID:       uuid.NewString(),
		Stage:    StageRaw,
		Thread:   threadID,
		Title:    req.Title,
		Projects: req.ProjectIDs,
		Status:   "draft",
		Created:  now,
		Body:     req.Content,
	}

	relPath := artifactRelPath(artifact)
	artifact.Path = relPath

	note := renderArtifact(artifact)
	if err := s.vault.WriteNote(relPath, note); err != nil {
		return nil, fmt.Errorf("writing raw idea: %w", err)
	}
	return artifact, nil
}

// WriteArtifact persists an Artifact to the vault with the correct layout.
// Used by the advance flow after Claude has generated the next stage.
func (s *Store) WriteArtifact(a *Artifact) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.Created.IsZero() {
		a.Created = time.Now()
	}
	if a.Status == "" {
		a.Status = "draft"
	}
	a.Path = artifactRelPath(a)

	note := renderArtifact(a)
	return s.vault.WriteNote(a.Path, note)
}

// GetThread returns every artifact in a thread, ordered by stage.
//
// NT-03: Thread.Projects is the UNION of project IDs across every
// artifact in the thread, not just the raw idea's list. When the pitch
// stage introduces a new project association (reasonable during
// shape/breadboard refinement), it surfaces in the thread summary
// instead of being silently hidden behind the original raw idea's
// projects. Union preserves backward-compat with threads whose later
// stages inherit the same projects.
func (s *Store) GetThread(threadID string) (*Thread, error) {
	all, err := s.walkArtifacts()
	if err != nil {
		return nil, err
	}

	var matched []*Artifact
	for _, a := range all {
		if a.Thread == threadID {
			matched = append(matched, a)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return stageOrder(matched[i].Stage) < stageOrder(matched[j].Stage)
	})

	latest := matched[len(matched)-1]
	return &Thread{
		ID:           threadID,
		Title:        matched[0].Title,
		Projects:     unionArtifactProjects(matched),
		CurrentStage: latest.Stage,
		Artifacts:    matched,
	}, nil
}

// unionArtifactProjects returns the distinct project IDs across a set of
// artifacts, preserving the order in which each ID is first seen. Used
// by GetThread and ListThreads so a thread's top-level `Projects` field
// reflects every project that any stage touches, not just the raw idea.
func unionArtifactProjects(arts []*Artifact) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, a := range arts {
		for _, pid := range a.Projects {
			if pid == "" || seen[pid] {
				continue
			}
			seen[pid] = true
			out = append(out, pid)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ListThreads groups all artifacts by thread UUID and returns one Thread
// summary per unique ID, newest thread first.
func (s *Store) ListThreads() ([]*Thread, error) {
	all, err := s.walkArtifacts()
	if err != nil {
		return nil, err
	}

	byThread := make(map[string][]*Artifact)
	for _, a := range all {
		if a.Thread == "" {
			continue
		}
		byThread[a.Thread] = append(byThread[a.Thread], a)
	}

	threads := make([]*Thread, 0, len(byThread))
	for id, arts := range byThread {
		sort.SliceStable(arts, func(i, j int) bool {
			return stageOrder(arts[i].Stage) < stageOrder(arts[j].Stage)
		})
		latest := arts[len(arts)-1]
		threads = append(threads, &Thread{
			ID:           id,
			Title:        arts[0].Title,
			// NT-03: union of every artifact's projects so later-stage
			// additions are visible, not just the raw idea's list.
			Projects:     unionArtifactProjects(arts),
			CurrentStage: latest.Stage,
			Artifacts:    arts,
		})
	}
	sort.SliceStable(threads, func(i, j int) bool {
		if len(threads[i].Artifacts) == 0 || len(threads[j].Artifacts) == 0 {
			return false
		}
		return threads[i].Artifacts[0].Created.After(threads[j].Artifacts[0].Created)
	})
	return threads, nil
}

// walkArtifacts globs vault/product/**/*.md and parses each file into an
// Artifact.
func (s *Store) walkArtifacts() ([]*Artifact, error) {
	root := filepath.Join(s.vault.Path(), productDir)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var artifacts []*Artifact
	for _, stage := range AllStages() {
		dir := filepath.Join(root, string(stage))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			relPath := filepath.ToSlash(filepath.Join(productDir, string(stage), e.Name()))
			a, err := s.readArtifact(relPath)
			if err != nil {
				slog.Warn("shapeup: skipping artifact", "path", relPath, "error", err)
				continue
			}
			artifacts = append(artifacts, a)
		}
	}
	return artifacts, nil
}

func (s *Store) readArtifact(relPath string) (*Artifact, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}
	a := &Artifact{Path: relPath}
	if id, ok := note.Frontmatter["id"].(string); ok {
		a.ID = id
	}
	if stage, ok := note.Frontmatter["stage"].(string); ok {
		a.Stage = Stage(stage)
	}
	if thread, ok := note.Frontmatter["thread"].(string); ok {
		a.Thread = thread
	}
	if parent, ok := note.Frontmatter["parent_id"].(string); ok {
		a.ParentID = parent
	}
	if title, ok := note.Frontmatter["title"].(string); ok {
		a.Title = title
	}
	if appetite, ok := note.Frontmatter["appetite"].(string); ok {
		a.Appetite = appetite
	}
	if status, ok := note.Frontmatter["status"].(string); ok {
		a.Status = status
	}
	if projects, ok := note.Frontmatter["projects"].([]interface{}); ok {
		for _, p := range projects {
			if s, ok := p.(string); ok {
				a.Projects = append(a.Projects, s)
			}
		}
	}
	if created, ok := note.Frontmatter["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			a.Created = t
		}
	}
	a.Body = note.Body
	return a, nil
}

// artifactRelPath returns the relative vault path for an artifact based on
// its stage, date, title, and a short suffix derived from the artifact ID.
//
// The ID suffix is what makes the path collision-proof: before HI-04 the
// filename was just `<date>_<slug>.md`, which silently clobbered whenever
// two raw ideas happened to share a title on the same day (the
// ShapeUpIdeasDestination emits one raw idea per extracted idea, so
// re-ingests and multi-idea documents hit this collision routinely). We
// derive the suffix from `a.ID` (the artifact UUID) rather than `a.Thread`
// so every artifact in a thread — raw, frame, shape, breadboard, pitch —
// lands on a distinct filename even though they share a thread UUID.
//
// Backward compatibility: `walkArtifacts` reads every `.md` file under the
// stage directory without parsing the filename, so existing files written
// with the old `<date>_<slug>.md` format continue to read back correctly
// alongside the new format. No migration is required.
//
// Callers must ensure `a.ID` is populated before calling this (both
// CreateRawIdea and WriteArtifact set ID before calling us). If ID is
// empty we fall back to a stable "noid" placeholder rather than panic,
// so a misuse produces an inspectable on-disk artifact instead of a
// crash — but the calling path is expected to assign a UUID first.
func artifactRelPath(a *Artifact) string {
	date := a.Created.Format("2006-01-02")
	slug := vault.Slugify(a.Title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}
	idShort := shortArtifactID(a.ID)
	return filepath.ToSlash(filepath.Join(productDir, string(a.Stage), fmt.Sprintf("%s_%s_%s.md", date, slug, idShort)))
}

// shortArtifactID returns the first 8 characters of an artifact UUID for
// use as a filename suffix. If the ID is shorter than 8 chars (should
// never happen for a real uuid.NewString() output but defends against
// tests / misuse) we return the whole ID or a placeholder.
func shortArtifactID(id string) string {
	if id == "" {
		return "noid"
	}
	if len(id) < 8 {
		return id
	}
	return id[:8]
}

// renderArtifact turns an Artifact into a vault.Note. The renderer is simple
// because the actual section-by-section rendering happens in the pipeline
// templates before the Body is set.
func renderArtifact(a *Artifact) *vault.Note {
	fm := map[string]interface{}{
		"id":      a.ID,
		"type":    "shapeup",
		"stage":   string(a.Stage),
		"thread":  a.Thread,
		"title":   a.Title,
		"status":  a.Status,
		"created": a.Created.Format(time.RFC3339),
	}
	if a.ParentID != "" {
		fm["parent_id"] = a.ParentID
	}
	if len(a.Projects) > 0 {
		fm["projects"] = a.Projects
	}
	if a.Appetite != "" {
		fm["appetite"] = a.Appetite
	}
	fm["tags"] = []string{"cortical", "shapeup", string(a.Stage)}

	body := a.Body
	if body == "" {
		body = fmt.Sprintf("# %s\n\n*(empty)*\n", a.Title)
	}
	return &vault.Note{Frontmatter: fm, Body: body}
}

func stageOrder(s Stage) int {
	for i, v := range AllStages() {
		if v == s {
			return i
		}
	}
	return -1
}
