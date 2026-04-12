package pipeline

import "testing"

// fakeTransformer is a test double whose name and CanHandle verdict
// are both injected.
type fakeTransformer struct {
	name    string
	handles bool
}

func (f *fakeTransformer) Name() string                         { return f.name }
func (f *fakeTransformer) CanHandle(input *RawInput) bool       { return f.handles }
func (f *fakeTransformer) Transform(input *RawInput) (*TextDocument, error) {
	return &TextDocument{Source: f.name}, nil
}

func TestFindTransformer(t *testing.T) {
	tests := []struct {
		name         string
		transformers []Transformer
		wantName     string
		wantNil      bool
	}{
		{
			name: "first match wins",
			transformers: []Transformer{
				&fakeTransformer{name: "a", handles: true},
				&fakeTransformer{name: "b", handles: true},
			},
			wantName: "a",
		},
		{
			name: "skips non-matching",
			transformers: []Transformer{
				&fakeTransformer{name: "a", handles: false},
				&fakeTransformer{name: "b", handles: true},
			},
			wantName: "b",
		},
		{
			name: "none match returns nil",
			transformers: []Transformer{
				&fakeTransformer{name: "a", handles: false},
				&fakeTransformer{name: "b", handles: false},
			},
			wantNil: true,
		},
		{
			name:         "empty list returns nil",
			transformers: nil,
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindTransformer(tt.transformers, &RawInput{})
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %s", got.Name())
				}
				return
			}
			if got == nil {
				t.Fatal("expected a transformer, got nil")
			}
			if got.Name() != tt.wantName {
				t.Errorf("got %q, want %q", got.Name(), tt.wantName)
			}
		})
	}
}
