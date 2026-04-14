package transformers

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestIsRequestURLSafe covers the pure in-process URL classifier used by
// the chromedp request interceptor. This is the single source of truth for
// "is this URL safe for the browser to hit", so every edge case that the
// interceptor might see from a real page belongs here.
func TestIsRequestURLSafe(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantBlock bool
		wantMatch string // substring expected in error if blocked
	}{
		// Browser-internal schemes must NOT be blocked — blocking
		// about:blank breaks chromedp's page init handshake.
		{"about:blank allowed", "about:blank", false, ""},
		{"data URL allowed", "data:text/html,<h1>ok</h1>", false, ""},
		{"blob URL allowed", "blob:https://example.com/abc", false, ""},

		// Non-network schemes must be blocked.
		{"file URL blocked", "file:///etc/passwd", true, "blocked scheme"},
		{"ftp blocked", "ftp://example.com/x", true, "blocked scheme"},
		{"chrome extension blocked", "chrome-extension://abc/", true, "blocked scheme"},
		{"javascript URL blocked", "javascript:alert(1)", true, "blocked scheme"},

		// Literal IP targets in the private / loopback ranges must be
		// blocked WITHOUT touching DNS (DNS rebinding safety).
		{"loopback v4 literal", "http://127.0.0.1:8000/api", true, "private IP literal"},
		{"loopback v4 high port literal", "http://127.0.0.1:1/", true, "private IP literal"},
		{"rfc1918 10.x literal", "http://10.0.0.5/api", true, "private IP literal"},
		{"rfc1918 192.168 literal", "https://192.168.1.100:3000/", true, "private IP literal"},
		{"rfc1918 172.16 literal", "http://172.16.0.1/", true, "private IP literal"},
		{"link-local v4 literal", "http://169.254.169.254/latest/meta-data/", true, "private IP literal"},
		{"cgnat literal", "http://100.64.0.1/", true, "private IP literal"},
		{"ipv6 loopback literal", "http://[::1]:8000/", true, "private IP literal"},
		{"ipv6 link-local literal", "http://[fe80::1]/", true, "private IP literal"},
		{"ipv6 ULA literal", "http://[fd12:3456::1]/", true, "private IP literal"},
		{"unspecified v4", "http://0.0.0.0:80/", true, "private IP literal"},

		// Public IP literals should pass.
		{"public v4 literal", "http://8.8.8.8/", false, ""},
		{"public v6 literal", "http://[2001:4860:4860::8888]/", false, ""},

		// Missing host — malformed URL must fail closed.
		{"no host", "http:///nohost", true, "no host"},

		// ws/wss handled as http/https (host-checked).
		{"ws loopback literal", "ws://127.0.0.1:9999/", true, "private IP literal"},
		{"wss public literal", "wss://1.1.1.1/", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := isRequestURLSafe(tt.url)
			if tt.wantBlock && err == nil {
				t.Errorf("isRequestURLSafe(%q) = nil, want block", tt.url)
			}
			if !tt.wantBlock && err != nil {
				t.Errorf("isRequestURLSafe(%q) = %v, want nil", tt.url, err)
			}
			if tt.wantBlock && tt.wantMatch != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantMatch) {
					t.Errorf("isRequestURLSafe(%q) error = %q, want substring %q",
						tt.url, err.Error(), tt.wantMatch)
				}
			}
		})
	}
}

// TestIsRequestURLSafe_LocalhostName exercises the DNS-lookup branch with
// a hostname that should always exist (localhost resolves to loopback). The
// result must still be BLOCKED because loopback is private.
func TestIsRequestURLSafe_LocalhostName(t *testing.T) {
	err := isRequestURLSafe("http://localhost:8000/api")
	if err == nil {
		t.Fatalf("isRequestURLSafe(localhost) = nil, want block")
	}
	// Accept either DNS-result block or (on some resolvers) parse/no-IP errors.
	if !strings.Contains(err.Error(), "localhost") && !strings.Contains(err.Error(), "blocked") {
		t.Errorf("unexpected error kind: %v", err)
	}
}

// TestIsRequestURLSafe_UnresolvableBlocks checks fail-closed behaviour: a
// host that cannot be resolved must be BLOCKED rather than allowed.
func TestIsRequestURLSafe_UnresolvableBlocks(t *testing.T) {
	// A TLD that definitely does not resolve. Using a random
	// non-existent subdomain of .invalid (RFC 2606 reserved).
	err := isRequestURLSafe("http://ssrf-guard-test-does-not-exist.invalid/")
	if err == nil {
		t.Fatalf("expected block for unresolvable host, got nil")
	}
}

// TestChromedpGet_PreflightBlocksLoopback verifies defense layer #1:
// chromedpGet refuses to even launch the browser when the target URL
// resolves to a private IP. This keeps the normal production call path
// honest regardless of what the interceptor does.
func TestChromedpGet_PreflightBlocksLoopback(t *testing.T) {
	// No need for a real server — validateSafeURL resolves and rejects
	// before any network call is made.
	_, err := chromedpGet("http://127.0.0.1:1/")
	if err == nil {
		t.Fatalf("chromedpGet(loopback) returned nil, want pre-flight block")
	}
	if !strings.Contains(err.Error(), "chromedp ssrf guard") {
		t.Errorf("unexpected error, want chromedp ssrf guard: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Integration test: real headless Chrome, real local HTTP servers.
//
// Strategy: normal production code refuses to launch chromedp against a
// loopback URL (defense #1). To actually exercise defense #2 (the in-browser
// request interceptor) we go through chromedpRender directly and use the
// package-private chromedpAllowTestURL hook to whitelist exactly one URL —
// the "public" test page. Every other request the page tries to make
// (including the "sensitive" loopback server it attacks) still runs through
// the real isRequestURLSafe check.
//
// Skipped automatically when Chrome/Chromium is not available.
// -----------------------------------------------------------------------------

func chromedpAvailable() bool {
	candidates := []string{
		"google-chrome",
		"chrome",
		"chromium",
		"chromium-browser",
		"chrome.exe",
		"msedge.exe",
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return true
		}
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	return false
}

// TestChromedpRender_InterceptorBlocksInPageSSRF wires up two loopback HTTP
// servers:
//
//   - publicSrv: serves an HTML page containing an <img>, a <script>, a
//     fetch() call, and an XHR — all targeting sensitiveSrv.
//   - sensitiveSrv: records every request; zero is the only acceptable count.
//
// We whitelist publicSrv's URL via chromedpAllowTestURL so the interceptor
// lets the initial page load. Every sub-resource / JS request then hits the
// real interceptor branch and must be blocked because sensitiveSrv is on
// loopback.
func TestChromedpRender_InterceptorBlocksInPageSSRF(t *testing.T) {
	if !chromedpAvailable() {
		t.Skip("chromedp integration test skipped: Chrome/Chromium/Edge binary not found on PATH or well-known locations")
	}
	if testing.Short() {
		t.Skip("chromedp integration test skipped in -short mode")
	}

	// Sensitive server — any hit means the defense is broken.
	var sensitiveHits int64
	sensitive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&sensitiveHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SECRET"))
	}))
	defer sensitive.Close()

	// Public server — serves a page whose sub-resources / JS attempt to reach
	// the sensitive server.
	sensURL := sensitive.URL
	publicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only the root is the test page. Everything else is a sub-resource
		// hit we didn't expect — log it so debugging is easier.
		if r.URL.Path != "/" {
			t.Logf("public server received unexpected sub-path: %s", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := fmt.Sprintf(`<!doctype html>
<html><head><title>ssrf test</title></head>
<body>
  <h1>ssrf test</h1>
  <img src="%s/via-img" alt="x">
  <script src="%s/via-script"></script>
  <script>
    try { fetch("%s/via-fetch"); } catch(e) {}
    try {
      var x = new XMLHttpRequest();
      x.open("GET", "%s/via-xhr", true);
      x.send();
    } catch(e) {}
  </script>
</body></html>`, sensURL, sensURL, sensURL, sensURL)
		_, _ = w.Write([]byte(html))
	})
	public := httptest.NewServer(publicHandler)
	defer public.Close()

	// Whitelist ONLY the exact public root URL so the initial navigation
	// succeeds. Every other URL (including the sensitive server) still
	// flows through the real interceptor which will block it.
	allowed := public.URL + "/"
	chromedpAllowTestURL = func(raw string) bool {
		// Normalize trailing-slash: chromedp may see "http://.../" or
		// "http://..." depending on how Navigate formats it.
		return raw == allowed || raw == public.URL
	}
	t.Cleanup(func() { chromedpAllowTestURL = nil })

	// chromedpRender bypasses the pre-flight validateSafeURL so this
	// loopback test target is reachable. Production callers use
	// chromedpGet, which applies both layers.
	_, renderErr := chromedpRender(public.URL + "/")
	if renderErr != nil {
		// Render may error because blocked sub-resources cause page-load
		// warnings, or because the interceptor causes partial loads. The
		// contract under test is "sensitive server was never hit", so we
		// only log this.
		t.Logf("chromedpRender returned error (non-fatal): %v", renderErr)
	}

	// Give any in-flight JS fetches a moment to hit the server if the
	// defense failed (belt-and-braces — if the defense works there are
	// zero hits, and the sleep just delays the assertion).
	time.Sleep(200 * time.Millisecond)

	if got := atomic.LoadInt64(&sensitiveHits); got != 0 {
		t.Fatalf("sensitive server was hit %d time(s); SSRF defense failed", got)
	}
}

// TestChromedpAllowTestURL_NilInProduction is a paranoia guard: the test
// hook MUST be nil when no test has set it. If another test leaks the hook
// across runs this would detect it on fresh process start (when no tests
// have touched it yet). In practice t.Cleanup in the integration test
// above handles reset, but this documents the invariant.
func TestChromedpAllowTestURL_NilInProduction(t *testing.T) {
	// This only meaningfully checks on the first test in the file; if other
	// tests ran first they should have cleaned up. The test is still a
	// useful regression tripwire.
	if chromedpAllowTestURL != nil {
		// Reset it so the rest of the suite runs cleanly, but fail the
		// test to flag the leak.
		chromedpAllowTestURL = nil
		t.Fatal("chromedpAllowTestURL was non-nil at test start; a previous test leaked it")
	}
}

// Compile-time check that the net package is used — tests would otherwise
// fail to compile if a refactor drops the last use. Pure documentation.
var _ = net.ParseIP
