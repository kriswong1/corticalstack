package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// SetEnvAndPersist sets an environment variable for the running process
// and persists it to the .env file so it survives restarts. If the .env
// file doesn't exist it is created. Existing keys are overwritten;
// other keys are preserved.
func SetEnvAndPersist(key, value string) error {
	// Set for the running process immediately.
	if err := os.Setenv(key, value); err != nil {
		return err
	}

	// Find .env: prefer cwd, fall back to executable dir.
	envPath := ".env"
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if ex, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(ex), ".env")
			if _, err := os.Stat(candidate); err == nil {
				envPath = candidate
			}
		}
	}

	// Read existing entries (empty map if file doesn't exist).
	existing, _ := godotenv.Read(envPath)
	if existing == nil {
		existing = make(map[string]string)
	}
	existing[key] = value

	return godotenv.Write(existing, envPath)
}
