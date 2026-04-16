package integrations

import (
	"fmt"
	"os"

	"github.com/kriswong/corticalstack/internal/config"
)

// ObsidianIntegration checks whether the configured vault path exists
// and is a directory. Unlike Deepgram, there is no remote service to
// health-check — the "test" is a local stat.
type ObsidianIntegration struct{}

func (o *ObsidianIntegration) ID() string   { return "obsidian" }
func (o *ObsidianIntegration) Name() string { return "Obsidian" }

func (o *ObsidianIntegration) Configured() bool {
	p := config.VaultPath()
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func (o *ObsidianIntegration) HealthCheck() error {
	p := config.VaultPath()
	info, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("vault path %q: %w", p, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("vault path %q is not a directory", p)
	}
	return nil
}
