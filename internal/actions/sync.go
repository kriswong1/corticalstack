package actions

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Sync writes an action's canonical line to every location where it should
// exist: source note, every associated project's ACTION-ITEMS.md, and the
// central tracker. Existing lines with the same ID are replaced; new IDs
// are appended under the "## Open Items" marker.
//
// HI-03: the entire read-modify-write cycle is serialized on s.syncMu so
// that two concurrent Sync calls touching the same markdown file cannot
// race — the second sync observes the first's writes. This is a global
// serialization on markdown sync, which is acceptable for a local app.
// Purely in-memory readers (List/Get/CountByStatus) take s.mu and are
// unaffected by the sync lock.
func (s *Store) Sync(a *Action) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	if err := s.EnsureCentralFile(); err != nil {
		return err
	}
	line := FormatLine(a)

	locations := []string{s.CentralFilePath()}
	if a.SourceNote != "" {
		locations = append(locations, a.SourceNote)
	}
	for _, pid := range a.ProjectIDs {
		locations = append(locations, s.ProjectFilePath(pid))
	}

	for _, loc := range locations {
		if err := writeOrReplaceLine(s.vault.Path(), loc, a.ID, line); err != nil {
			// Non-fatal per location so one broken file doesn't block others.
			slog.Warn("actions.Sync: write failed", "location", loc, "error", err)
		}
	}
	return nil
}

// writeOrReplaceLine finds any existing line carrying this action's ID
// inside relPath and replaces it. If no match exists, appends the line
// under the "## Open Items" header (or at end of file if no header).
// Creates the file if it doesn't exist.
func writeOrReplaceLine(vaultPath, relPath, id, newLine string) error {
	full := filepath.Join(vaultPath, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}

	var content string
	data, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		content = string(data)
	}

	idMarker := "<!-- id:" + id + " -->"
	lines := strings.Split(content, "\n")
	replaced := false
	for i, line := range lines {
		if strings.Contains(line, idMarker) {
			lines[i] = newLine
			replaced = true
			break
		}
	}

	if !replaced {
		marker := "## Open Items"
		idx := strings.Index(content, marker)
		if idx >= 0 {
			// Append right after the marker line.
			markerLineEnd := idx + len(marker)
			// Skip to end of that line
			for markerLineEnd < len(content) && content[markerLineEnd] != '\n' {
				markerLineEnd++
			}
			// Build new content
			prefix := content[:markerLineEnd+1]
			suffix := content[markerLineEnd+1:]
			content = prefix + "\n" + newLine + "\n" + suffix
		} else {
			// Fall back: append to end
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += "\n" + newLine + "\n"
		}
		return os.WriteFile(full, []byte(content), 0o600)
	}

	return os.WriteFile(full, []byte(strings.Join(lines, "\n")), 0o600)
}
