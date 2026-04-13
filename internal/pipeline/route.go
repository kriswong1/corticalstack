package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/vault"
)

// --- Vault Note Destination ---

// VaultNoteDestination writes a structured Obsidian markdown note for every
// processed document. Body layout comes from the intention-specific template;
// folder placement comes from the document's source transformer.
type VaultNoteDestination struct {
	vault     *vault.Vault
	templates *TemplateRegistry
}

// NewVaultNoteDestination wires a destination with the default template registry.
func NewVaultNoteDestination(v *vault.Vault) *VaultNoteDestination {
	return &VaultNoteDestination{vault: v, templates: NewTemplateRegistry()}
}

func (d *VaultNoteDestination) Name() string { return "vault-note" }

func (d *VaultNoteDestination) Accept(doc *TextDocument, extracted *Extracted) (string, error) {
	date := docDateOrNow(doc)

	slug := vault.Slugify(doc.Title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}

	folder := sourceFolder(doc.Source)
	relPath := filepath.Join(folder, fmt.Sprintf("%s_%s.md", date, slug))

	intention := ""
	if extracted != nil {
		intention = extracted.Intention
	}
	renderer := d.templates.Pick(intention)

	fm, body := renderer.Render(doc, extracted)

	note := &vault.Note{Frontmatter: fm, Body: body}
	if err := d.vault.WriteNote(relPath, note); err != nil {
		return "", fmt.Errorf("writing vault note: %w", err)
	}
	// Stash the canonical source path in the doc so downstream destinations
	// (action store, daily log) can reference it by path.
	if doc.Metadata == nil {
		doc.Metadata = map[string]string{}
	}
	doc.Metadata["note_path"] = filepath.ToSlash(relPath)
	return filepath.ToSlash(relPath), nil
}

// --- Action Items Destination ---

// ActionItemsDestination upserts extracted actions into the smart action store
// and syncs them across the central tracker, source note, and per-project files.
type ActionItemsDestination struct {
	vault *vault.Vault
	store *actions.Store
}

// NewActionItemsDestination wires a destination bound to an action store.
// The store is optional — passing nil falls back to the v1 legacy writer.
func NewActionItemsDestination(v *vault.Vault, store *actions.Store) *ActionItemsDestination {
	return &ActionItemsDestination{vault: v, store: store}
}

func (d *ActionItemsDestination) Name() string { return "action-items" }

func (d *ActionItemsDestination) Accept(doc *TextDocument, extracted *Extracted) (string, error) {
	if extracted == nil || len(extracted.Actions) == 0 {
		return "", nil
	}

	if d.store == nil {
		// Legacy path: just append to the central file without tracking.
		date := docDateOrNow(doc)
		if err := d.vault.AppendActionItems(doc.Title, date, extracted.Actions); err != nil {
			return "", err
		}
		return "ACTION-ITEMS.md", nil
	}

	sourceNote := doc.Metadata["note_path"]
	projectIDs := splitAndTrim(doc.Metadata["projects"], ",")

	for _, raw := range extracted.Actions {
		a := &actions.Action{
			ID:          raw.ID,
			Title:       raw.Title,
			Description: raw.Description,
			Owner:       raw.Owner,
			Deadline:    raw.Deadline,
			Status:      actions.StatusInbox,
			Priority:    actions.Priority(raw.Priority),
			Effort:      actions.Effort(raw.Effort),
			Context:     raw.Context,
			SourceNote:  sourceNote,
			SourceTitle: doc.Title,
			ProjectIDs:  projectIDs,
		}
		stored, err := d.store.Upsert(a)
		if err != nil {
			return "", fmt.Errorf("upsert action: %w", err)
		}
		if err := d.store.Sync(stored); err != nil {
			return "", fmt.Errorf("sync action %s: %w", stored.ID, err)
		}
	}
	return d.store.CentralFilePath(), nil
}

// --- Daily Log Destination ---

type DailyLogDestination struct {
	vault *vault.Vault
}

func NewDailyLogDestination(v *vault.Vault) *DailyLogDestination {
	return &DailyLogDestination{vault: v}
}

func (d *DailyLogDestination) Name() string { return "daily-log" }

func (d *DailyLogDestination) Accept(doc *TextDocument, extracted *Extracted) (string, error) {
	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("[ingest:%s] \"%s\"", doc.Source, doc.Title))

	if extracted != nil {
		if extracted.Intention != "" {
			entry.WriteString(fmt.Sprintf(" (%s)", extracted.Intention))
		}
		counts := []string{}
		if len(extracted.Actions) > 0 {
			counts = append(counts, fmt.Sprintf("%d actions", len(extracted.Actions)))
		}
		if len(extracted.KeyPoints) > 0 {
			counts = append(counts, fmt.Sprintf("%d key points", len(extracted.KeyPoints)))
		}
		if len(counts) > 0 {
			entry.WriteString(fmt.Sprintf(". %s", strings.Join(counts, ", ")))
		}
	}

	err := d.vault.AppendToDaily(entry.String())
	return "daily/" + time.Now().Format("2006-01-02") + ".md", err
}

// --- Helpers ---

// sourceFolder picks the top-level vault folder per document source.
func sourceFolder(source string) string {
	switch source {
	case "deepgram", "audio":
		return "audio"
	case "vtt":
		return "transcripts"
	case "pdf", "docx":
		return "documents"
	case "youtube":
		return "videos"
	case "webpage", "html", "linkedin":
		return "articles"
	case "passthrough":
		return "notes"
	default:
		return "inbox"
	}
}

// EnsureVaultFolders creates the standard folder layout used by route.go.
func EnsureVaultFolders(v *vault.Vault) {
	folders := []string{"notes", "audio", "transcripts", "documents", "articles", "videos", "inbox", "daily", "projects"}
	for _, f := range folders {
		_ = os.MkdirAll(filepath.Join(v.Path(), f), 0o700)
	}
}
