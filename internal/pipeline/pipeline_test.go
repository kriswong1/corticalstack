package pipeline

import (
	"strings"
	"testing"
)

func TestExtractionConfigForSource(t *testing.T) {
	fullSources := []string{"deepgram", "audio", "pdf", "docx", "youtube", "webpage", "linkedin", "html"}
	for _, src := range fullSources {
		cfg := extractionConfigForSource(src)
		if !cfg.ExtractSummary || !cfg.ExtractDecisions || !cfg.ExtractActions || !cfg.ExtractIdeas || !cfg.ExtractLearnings {
			t.Errorf("source %q should get default (full) config, got %+v", src, cfg)
		}
	}

	unknownSources := []string{"", "unknown", "text", "random"}
	for _, src := range unknownSources {
		cfg := extractionConfigForSource(src)
		if !cfg.ExtractSummary || !cfg.ExtractActions {
			t.Errorf("source %q should get light config with summary+actions, got %+v", src, cfg)
		}
		if cfg.ExtractDecisions || cfg.ExtractIdeas || cfg.ExtractLearnings {
			t.Errorf("source %q should NOT have extra extractions in light config, got %+v", src, cfg)
		}
	}
}

func TestDefaultConfigForSource(t *testing.T) {
	// Should be a pass-through to extractionConfigForSource.
	a := DefaultConfigForSource("pdf")
	b := extractionConfigForSource("pdf")
	if a.ExtractSummary != b.ExtractSummary || a.ExtractActions != b.ExtractActions ||
		a.ExtractDecisions != b.ExtractDecisions || a.ExtractIdeas != b.ExtractIdeas ||
		a.ExtractLearnings != b.ExtractLearnings {
		t.Errorf("DefaultConfigForSource and extractionConfigForSource disagree: %+v vs %+v", a, b)
	}
}

func TestDefaultExtractionConfig(t *testing.T) {
	cfg := DefaultExtractionConfig()
	if !cfg.ExtractSummary || !cfg.ExtractDecisions || !cfg.ExtractActions || !cfg.ExtractIdeas || !cfg.ExtractLearnings {
		t.Errorf("DefaultExtractionConfig should enable everything, got %+v", cfg)
	}
}

func TestLightExtractionConfig(t *testing.T) {
	cfg := LightExtractionConfig()
	if !cfg.ExtractSummary || !cfg.ExtractActions {
		t.Errorf("LightExtractionConfig should enable summary and actions, got %+v", cfg)
	}
	if cfg.ExtractDecisions || cfg.ExtractIdeas || cfg.ExtractLearnings {
		t.Errorf("LightExtractionConfig should NOT enable decisions/ideas/learnings, got %+v", cfg)
	}
}

func TestFormatResult(t *testing.T) {
	tests := []struct {
		name     string
		r        *ProcessResult
		wantHas  []string
	}{
		{
			name: "successful result",
			r: &ProcessResult{
				Document:    &TextDocument{Title: "My Doc", Content: strings.Repeat("a", 1234)},
				Transformer: "webpage",
				Outputs:     map[string]string{"vault-note": "notes/my-doc.md"},
			},
			wantHas: []string{"My Doc", "webpage", "1234 chars", "vault-note", "notes/my-doc.md"},
		},
		{
			name: "result with errors",
			r: &ProcessResult{
				Document:    &TextDocument{Title: "Err Doc"},
				Transformer: "pdf",
				Errors:      []string{"extraction failed: bad input"},
			},
			wantHas: []string{"Err Doc", "pdf", "Errors:", "extraction failed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatResult(tt.r)
			for _, sub := range tt.wantHas {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing %q\nfull: %q", sub, got)
				}
			}
		})
	}
}
