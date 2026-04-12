package vault

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello World", "hello-world"},
		{"Already-Slugified-123", "already-slugified-123"},
		{"MixedCASE", "mixedcase"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"multiple   spaces", "multiple-spaces"},
		{"under_score_test", "under-score-test"},
		{"punctuation!!! matters???", "punctuation-matters"},
		{"emoji 🌍 test", "emoji-test"},
		{"日本語", ""},
		{"", ""},
		{"---", ""},
		{"dash--in--middle", "dash-in-middle"},
		{"a1b2c3", "a1b2c3"},
		{"   ", ""},
		{"hello_world foo-bar BAZ", "hello-world-foo-bar-baz"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := Slugify(tt.in)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
