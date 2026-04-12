package transformers

import (
	"net"
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		// Loopback
		{"127.0.0.1", true},
		{"127.255.255.254", true},
		{"::1", true},

		// RFC 1918
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.254", true},
		{"192.168.0.1", true},
		{"192.168.1.100", true},

		// Link-local
		{"169.254.1.1", true},
		{"fe80::1", true},

		// CGNAT (RFC 6598)
		{"100.64.0.1", true},
		{"100.127.255.255", true},

		// IPv6 ULA
		{"fc00::1", true},
		{"fd12:3456::1", true},

		// Unspecified
		{"0.0.0.0", true},
		{"::", true},

		// Multicast
		{"224.0.0.1", true},
		{"ff02::1", true},

		// Public / routable
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"199.232.1.1", false},
		{"2001:4860:4860::8888", false},
		{"100.63.255.255", false}, // just below CGNAT
		{"100.128.0.0", false},    // just above CGNAT
		{"172.15.255.255", false}, // just below RFC 1918
		{"172.32.0.0", false},     // just above RFC 1918
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("ParseIP(%q) returned nil", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantHas []string
		wantNot []string
	}{
		{
			name:    "removes script tags",
			in:      "before<script>alert('xss')</script>after",
			wantHas: []string{"before", "after"},
			wantNot: []string{"alert", "<script>"},
		},
		{
			name:    "removes style tags",
			in:      "before<style>body{color:red}</style>after",
			wantHas: []string{"before", "after"},
			wantNot: []string{"color:red", "<style>"},
		},
		{
			name:    "removes noscript tags",
			in:      "a<noscript>fallback</noscript>b",
			wantHas: []string{"a", "b"},
			wantNot: []string{"fallback"},
		},
		{
			name:    "strips generic tags",
			in:      "<p>hello <b>world</b></p>",
			wantHas: []string{"hello", "world"},
			wantNot: []string{"<p>", "<b>"},
		},
		{
			name:    "decodes common entities",
			in:      "<p>a &amp; b &lt;c&gt; &quot;d&quot;</p>",
			wantHas: []string{"a & b", "<c>", "\"d\""},
		},
		{
			name:    "collapses whitespace",
			in:      "<p>one   two\t\tthree</p>",
			wantHas: []string{"one two three"},
		},
		{
			name:    "empty input",
			in:      "",
			wantHas: []string{},
		},
		{
			name:    "plain text passes through",
			in:      "just plain text",
			wantHas: []string{"just plain text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.in)
			for _, sub := range tt.wantHas {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing %q\nfull: %q", sub, got)
				}
			}
			for _, sub := range tt.wantNot {
				if strings.Contains(got, sub) {
					t.Errorf("output should not contain %q\nfull: %q", sub, got)
				}
			}
		})
	}
}

func TestExtractHTMLTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple title", "<html><head><title>Hello World</title></head></html>", "Hello World"},
		{"no title tag", "<html><head></head></html>", ""},
		{"empty title", "<title></title>", ""},
		{"title with entities", "<title>Tom &amp; Jerry</title>", "Tom & Jerry"},
		{"title with whitespace", "<title>  Trimmed  </title>", "Trimmed"},
		{"title with attributes", `<title lang="en">Attr Title</title>`, "Attr Title"},
		{"first of multiple", "<title>First</title><title>Second</title>", "First"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHTMLTitle(tt.in)
			if got != tt.want {
				t.Errorf("extractHTMLTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeCommonEntities(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"a &amp; b", "a & b"},
		{"&lt;tag&gt;", "<tag>"},
		{"&quot;quoted&quot;", "\"quoted\""},
		{"it&#39;s", "it's"},
		{"a&nbsp;b", "a b"},
		{"no entities here", "no entities here"},
		{"", ""},
		{"&amp;&lt;&gt;", "&<>"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := decodeCommonEntities(tt.in)
			if got != tt.want {
				t.Errorf("decodeCommonEntities(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIdentifierFor(t *testing.T) {
	tests := []struct {
		name  string
		input *pipeline.RawInput
		want  string // "" means "any non-empty"
	}{
		{
			name:  "filename wins",
			input: &pipeline.RawInput{Filename: "doc.pdf", Path: "/tmp/x.pdf", URL: "https://e.com"},
			want:  "doc.pdf",
		},
		{
			name:  "path when no filename",
			input: &pipeline.RawInput{Path: "/tmp/somefile.md"},
			want:  "somefile.md",
		},
		{
			name:  "url when no filename/path",
			input: &pipeline.RawInput{URL: "https://example.com/article"},
			want:  "https://example.com/article",
		},
		{
			name:  "fallback to timestamp",
			input: &pipeline.RawInput{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identifierFor(tt.input)
			if tt.want == "" {
				if !strings.HasPrefix(got, "text-") {
					t.Errorf("expected fallback prefix 'text-', got %q", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("identifierFor(%+v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMergeMeta(t *testing.T) {
	tests := []struct {
		name       string
		base       map[string]string
		extra      map[string]string
		wantKey    string
		wantValue  string
		wantLength int
	}{
		{
			name:       "extra overrides base",
			base:       map[string]string{"k": "base", "other": "b"},
			extra:      map[string]string{"k": "extra"},
			wantKey:    "k",
			wantValue:  "extra",
			wantLength: 2,
		},
		{
			name:       "nil base ok",
			base:       nil,
			extra:      map[string]string{"a": "1"},
			wantKey:    "a",
			wantValue:  "1",
			wantLength: 1,
		},
		{
			name:       "nil extra ok",
			base:       map[string]string{"b": "2"},
			extra:      nil,
			wantKey:    "b",
			wantValue:  "2",
			wantLength: 1,
		},
		{
			name:       "both nil",
			base:       nil,
			extra:      nil,
			wantLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeMeta(tt.base, tt.extra)
			if len(got) != tt.wantLength {
				t.Errorf("length: got %d, want %d (map: %v)", len(got), tt.wantLength, got)
			}
			if tt.wantKey != "" && got[tt.wantKey] != tt.wantValue {
				t.Errorf("got[%q] = %q, want %q", tt.wantKey, got[tt.wantKey], tt.wantValue)
			}
		})
	}
}
