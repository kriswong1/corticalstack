// Package platform provides OS and environment detection helpers so the
// same CorticalStack binary can run cleanly on native Linux, native Windows,
// and WSL2 (Linux hosted by Windows) without per-caller branching.
package platform

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

var (
	wslOnce sync.Once
	wslVal  bool
)

// IsWSL reports whether the process is running inside a WSL2 environment.
// The result is cached after the first call. Set CORTICAL_WSL=1 to force
// detection on in environments where the env hints are stripped (useful
// for tests and for users running under minimal init systems).
func IsWSL() bool {
	wslOnce.Do(func() { wslVal = detectWSL() })
	return wslVal
}

func detectWSL() bool {
	if os.Getenv("CORTICAL_WSL") == "1" {
		return true
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	if os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	if b, err := os.ReadFile("/proc/version"); err == nil {
		lower := strings.ToLower(string(b))
		if strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl") {
			return true
		}
	}
	return false
}

// driveLetterRE matches a drive-letter prefix like "C:\" or "d:/".
// A separator is required so we don't accidentally rewrite Windows
// drive-relative paths like "C:foo" (which are unusable from WSL anyway).
var driveLetterRE = regexp.MustCompile(`^([A-Za-z]):[\\/]`)

// TranslateWindowsPath rewrites a Windows drive-letter path to its WSL2
// /mnt/<drive>/ equivalent and normalizes separators to forward slashes.
// Paths that are already POSIX, relative, or empty are returned unchanged,
// so the function is safe to call on any input regardless of platform.
//
//	C:\Users\kris\vault -> /mnt/c/Users/kris/vault
//	D:/data/notes       -> /mnt/d/data/notes
//	/home/kris/vault    -> /home/kris/vault
//	vault               -> vault
func TranslateWindowsPath(p string) string {
	m := driveLetterRE.FindStringSubmatch(p)
	if m == nil {
		return p
	}
	drive := strings.ToLower(m[1])
	rest := p[len(m[0]):]
	rest = strings.ReplaceAll(rest, `\`, `/`)
	return "/mnt/" + drive + "/" + rest
}

// MaybeTranslateForWSL is a convenience wrapper that only rewrites drive
// letter paths when the process is actually running under WSL2. On native
// Windows or Linux it returns the input unchanged.
func MaybeTranslateForWSL(p string) string {
	if !IsWSL() {
		return p
	}
	return TranslateWindowsPath(p)
}
