package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DailyLogPath returns the relative path for a given date's daily log.
func DailyLogPath(t time.Time) string {
	return filepath.Join("daily", t.Format("2006-01-02")+".md")
}

// TodayLogPath returns the relative path for today's daily log.
func TodayLogPath() string {
	return DailyLogPath(time.Now())
}

// EnsureDailyLog creates today's daily log if it doesn't exist.
func (v *Vault) EnsureDailyLog() error {
	relPath := TodayLogPath()
	if v.Exists(relPath) {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	note := &Note{
		Frontmatter: map[string]interface{}{
			"date": today,
			"type": "daily",
		},
		Body: fmt.Sprintf("# %s\n", today),
	}
	return v.WriteNote(relPath, note)
}

// AppendToDaily appends a timestamped entry to today's daily log.
func (v *Vault) AppendToDaily(entry string) error {
	if err := v.EnsureDailyLog(); err != nil {
		return err
	}
	fullPath := filepath.Join(v.path, TodayLogPath())
	f, err := os.OpenFile(fullPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening daily log: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n- **%s** %s\n", time.Now().Format("15:04"), entry)
	return err
}
