package persona

import (
	"fmt"
	"strings"
)

// BuildContextPrompt returns a single markdown block to prepend to any
// Claude prompt. It includes every persona file with content, trimmed to
// its per-file budget, in a stable SOUL → USER → MEMORY order.
//
// Files that are missing or empty are silently skipped — Claude calls
// remain functional even if the user has never created the vault files.
//
// NT-06: result is cached on the Loader so repeated calls are cheap.
// The underlying Loader.Get() is mtime-cached, so a file edit in
// Obsidian invalidates the file cache on the next Get; we detect the
// mtime change by re-running Get for every persona and comparing a
// cheap (mtime, len) fingerprint against the cached prompt's
// fingerprint. If anything differs, the prompt is rebuilt and the new
// fingerprint stashed. Set() already invalidates the underlying file
// cache, so an external write-through persona edit is picked up
// automatically on the next BuildContextPrompt call.
func (l *Loader) BuildContextPrompt() string {
	if l == nil {
		return ""
	}

	// Collect trimmed content + fingerprint for every persona file.
	contents := make(map[Name]string, len(AllNames()))
	fp := make(map[Name]int, len(AllNames()))
	for _, name := range AllNames() {
		c := l.getTrimmed(name)
		contents[name] = c
		// Fingerprint is just content length. BuildContextPrompt is a
		// pure function of the trimmed-content map, so if every length
		// matches and the mtime-cached Get returned a cached string,
		// the resulting prompt is unchanged.
		fp[name] = len(c)
	}

	// Check the cached prompt against the current fingerprint.
	l.mu.RLock()
	if l.promptCache != "" && fingerprintsEqual(l.promptFP, fp) {
		cached := l.promptCache
		l.mu.RUnlock()
		return cached
	}
	l.mu.RUnlock()

	var sections []string
	for _, name := range AllNames() {
		content := contents[name]
		if !hasBody(content) {
			continue
		}
		heading := fmt.Sprintf("## %s context\n\n%s", strings.ToUpper(string(name)), strings.TrimSpace(content))
		sections = append(sections, heading)
	}

	var prompt string
	if len(sections) > 0 {
		var b strings.Builder
		b.WriteString("# Persona context\n\n")
		b.WriteString("*The following context files tailor Claude's output to the user. Respect them unless the user's request contradicts them.*\n\n")
		b.WriteString(strings.Join(sections, "\n\n---\n\n"))
		b.WriteString("\n\n---\n\n")
		prompt = b.String()
	}

	l.mu.Lock()
	l.promptCache = prompt
	l.promptFP = fp
	l.mu.Unlock()

	return prompt
}

// ContextBuilder is the subset of *Loader that consumers call to get the
// persona prompt prefix. Every consumer (intent classifier, shapeup
// advancer, PRD/prototype/usecase synthesizers, pipeline extractor)
// holds this interface so constructors can substitute NoopContextBuilder
// when the caller passes a nil *Loader, instead of relying on a
// nil-receiver guard at every call site.
type ContextBuilder interface {
	BuildContextPrompt() string
}

// NoopContextBuilder returns an empty prompt prefix. Used by consumer
// constructors when persona context is disabled.
type NoopContextBuilder struct{}

// BuildContextPrompt on NoopContextBuilder always returns "".
func (NoopContextBuilder) BuildContextPrompt() string { return "" }

// ResolveContextBuilder returns p if non-nil, otherwise NoopContextBuilder{}.
// Consumer constructors use this to keep accepting *Loader as their public
// argument while storing a non-nil interface internally.
func ResolveContextBuilder(p *Loader) ContextBuilder {
	if p == nil {
		return NoopContextBuilder{}
	}
	return p
}

// fingerprintsEqual compares two name→length maps field-by-field. Used by
// BuildContextPrompt to decide whether the cached prompt is still valid.
func fingerprintsEqual(a, b map[Name]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
