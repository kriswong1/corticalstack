package transformers

import (
	"testing"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

func TestWebPageName(t *testing.T) {
	tr := &WebPageTransformer{}
	if got := tr.Name(); got != "webpage" {
		t.Errorf("Name() = %q, want %q", got, "webpage")
	}
}

func TestWebPageCanHandle(t *testing.T) {
	tr := &WebPageTransformer{}
	tests := []struct {
		name  string
		input pipeline.RawInput
		want  bool
	}{
		{
			name:  "https URL accepted",
			input: pipeline.RawInput{Kind: pipeline.InputURL, URL: "https://example.com/page"},
			want:  true,
		},
		{
			name:  "http URL accepted",
			input: pipeline.RawInput{Kind: pipeline.InputURL, URL: "http://example.com/page"},
			want:  true,
		},
		{
			name:  "InputText rejected",
			input: pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hello")},
			want:  false,
		},
		{
			name:  "InputFile rejected",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Filename: "page.html"},
			want:  false,
		},
		{
			name:  "empty URL rejected",
			input: pipeline.RawInput{Kind: pipeline.InputURL, URL: ""},
			want:  false,
		},
		{
			name:  "ftp URL rejected",
			input: pipeline.RawInput{Kind: pipeline.InputURL, URL: "ftp://files.example.com/doc"},
			want:  false,
		},
		{
			name:  "URL without scheme rejected",
			input: pipeline.RawInput{Kind: pipeline.InputURL, URL: "example.com/page"},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tr.CanHandle(&tt.input)
			if got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}
