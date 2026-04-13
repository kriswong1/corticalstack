package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/shapeup"
)

// --- ShapeUp Ideas Destination ---

// ShapeUpIdeasDestination converts every extracted idea into a raw ShapeUp
// artifact. Each idea starts a brand new thread at the `raw` stage so it
// surfaces in the ShapedPRD queue ready to be advanced to frame → shape →
// breadboard → pitch. Source linkage is preserved in the body so the user
// can always trace a raw idea back to the document it came from.
type ShapeUpIdeasDestination struct {
	store *shapeup.Store
}

// NewShapeUpIdeasDestination wires a destination bound to a ShapeUp store.
// Pass nil to disable raw-idea capture (useful for tests).
func NewShapeUpIdeasDestination(store *shapeup.Store) *ShapeUpIdeasDestination {
	return &ShapeUpIdeasDestination{store: store}
}

func (d *ShapeUpIdeasDestination) Name() string { return "shapeup-ideas" }

func (d *ShapeUpIdeasDestination) Accept(doc *TextDocument, extracted *Extracted) (string, error) {
	if d.store == nil || extracted == nil || len(extracted.Ideas) == 0 {
		return "", nil
	}

	sourceNote := ""
	if doc.Metadata != nil {
		sourceNote = doc.Metadata["note_path"]
	}
	projectIDs := splitAndTrim(safeMeta(doc, "projects"), ",")

	var lastPath string
	for _, idea := range extracted.Ideas {
		idea = strings.TrimSpace(idea)
		if idea == "" {
			continue
		}
		req := shapeup.CreateIdeaRequest{
			Title:      ideaTitle(idea),
			Content:    ideaBody(idea, doc, sourceNote),
			ProjectIDs: projectIDs,
		}
		art, err := d.store.CreateRawIdea(req)
		if err != nil {
			return "", fmt.Errorf("shapeup raw idea: %w", err)
		}
		lastPath = art.Path
	}
	return lastPath, nil
}

// ideaTitle truncates an idea sentence to a filename-friendly title.
func ideaTitle(idea string) string {
	title := strings.TrimSpace(idea)
	title = strings.TrimRight(title, ".!?")
	if len(title) > 80 {
		title = title[:80]
	}
	return title
}

// ideaBody renders the raw-idea markdown body with a source backlink so the
// user can navigate from the ShapeUp queue back to the original note.
func ideaBody(idea string, doc *TextDocument, sourceNote string) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(ideaTitle(idea))
	b.WriteString("\n\n")
	b.WriteString(idea)
	b.WriteString("\n\n---\n\n")
	if sourceNote != "" {
		b.WriteString(fmt.Sprintf("**Source:** [[%s]]", sourceNote))
	} else if doc.Title != "" {
		b.WriteString(fmt.Sprintf("**Source:** %s", doc.Title))
	}
	if doc.Source != "" {
		b.WriteString(fmt.Sprintf(" _(%s)_", doc.Source))
	}
	b.WriteString("  \n")
	b.WriteString(fmt.Sprintf("**Captured:** %s\n", time.Now().Format("2006-01-02")))
	return b.String()
}

func safeMeta(doc *TextDocument, key string) string {
	if doc == nil || doc.Metadata == nil {
		return ""
	}
	return doc.Metadata[key]
}
