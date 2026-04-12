package vault

import (
	"strings"
	"testing"
	"time"
)

func TestDailyLogPath(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{
			name: "mid-month date",
			t:    time.Date(2026, 4, 11, 15, 30, 0, 0, time.UTC),
			want: "daily/2026-04-11.md",
		},
		{
			name: "january first",
			t:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			want: "daily/2025-01-01.md",
		},
		{
			name: "december last",
			t:    time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC),
			want: "daily/2030-12-31.md",
		},
		{
			name: "leap day",
			t:    time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
			want: "daily/2024-02-29.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DailyLogPath(tt.t)
			// Accept either OS path separator since filepath.Join may
			// produce "\" on Windows.
			gotNormalized := strings.ReplaceAll(got, "\\", "/")
			if gotNormalized != tt.want {
				t.Errorf("DailyLogPath(%v) = %q, want %q", tt.t, got, tt.want)
			}
		})
	}
}

func TestTodayLogPath(t *testing.T) {
	got := TodayLogPath()
	gotNormalized := strings.ReplaceAll(got, "\\", "/")
	want := "daily/" + time.Now().Format("2006-01-02") + ".md"
	if gotNormalized != want {
		t.Errorf("TodayLogPath() = %q, want %q", got, want)
	}
}
