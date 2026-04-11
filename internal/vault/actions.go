package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const actionItemsFile = "ACTION-ITEMS.md"

// ActionItem represents a task extracted from a document.
// ID is assigned by the pipeline before rendering so templates can embed
// the stable identifier in the canonical markdown line.
type ActionItem struct {
	ID          string `json:"id,omitempty"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
	Deadline    string `json:"deadline,omitempty"`
}

// AppendActionItems adds action items to the central tracker,
// grouped under a heading identifying the source document.
func (v *Vault) AppendActionItems(sourceTitle string, sourceDate string, items []ActionItem) error {
	fullPath := filepath.Join(v.path, actionItemsFile)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		header := `---
type: tracker
purpose: Central action item tracker across all ingested documents
---

# Action Items

> All action items extracted by CorticalStack. Check off items as completed.
> Items are grouped by source, newest first.

## Open Items

`
		if err := os.WriteFile(fullPath, []byte(header), 0o600); err != nil {
			return fmt.Errorf("creating action items file: %w", err)
		}
	}

	var section strings.Builder
	section.WriteString(fmt.Sprintf("### From: %s (%s)\n", sourceTitle, sourceDate))
	for _, item := range items {
		owner := item.Owner
		if owner == "" {
			owner = "TBD"
		}
		deadline := ""
		if item.Deadline != "" {
			deadline = fmt.Sprintf(" *(due: %s)*", item.Deadline)
		}
		section.WriteString(fmt.Sprintf("- [ ] [%s] %s%s\n", owner, item.Description, deadline))
	}
	section.WriteString("\n")

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}

	contentStr := string(content)
	marker := "## Open Items\n"
	if idx := strings.Index(contentStr, marker); idx >= 0 {
		insertAt := idx + len(marker)
		for insertAt < len(contentStr) && contentStr[insertAt] == '\n' {
			insertAt++
		}
		contentStr = contentStr[:insertAt] + "\n" + section.String() + contentStr[insertAt:]
	} else {
		contentStr += "\n" + section.String()
	}

	return os.WriteFile(fullPath, []byte(contentStr), 0o600)
}
