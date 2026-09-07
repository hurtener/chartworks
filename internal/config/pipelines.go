package config

import (
	"encoding/hex"
	"path/filepath"
	"time"
)

// Pipelines configures the supervised managed-write runner. Path and SHA256 are
// operator-owned artifact coordinates; no definition can select an executable.
type Pipelines struct {
	Enabled        bool     `json:"enabled"`
	RunnerPath     string   `json:"runner_path"`
	RunnerVersion  string   `json:"runner_version"`
	RunnerSHA256   string   `json:"runner_sha256"`
	TempDir        string   `json:"temp_dir"`
	Timeout        Duration `json:"timeout"`
	Concurrency    int      `json:"concurrency"`
	MaxSteps       int      `json:"max_steps"`
	MaxSQLBytes    int      `json:"max_sql_bytes"`
	MaxOutputBytes int      `json:"max_output_bytes"`
}

func DefaultPipelines() Pipelines {
	return Pipelines{RunnerVersion: "v0.11.749", Timeout: Duration(45 * time.Second), Concurrency: 1, MaxSteps: 8, MaxSQLBytes: 65536, MaxOutputBytes: 1 << 20}
}
func ValidatePipelines(v Pipelines) error {
	if v.Timeout < Duration(time.Second) || v.Timeout > Duration(time.Minute) || v.Concurrency < 1 || v.Concurrency > 8 || v.MaxSteps < 1 || v.MaxSteps > 32 || v.MaxSQLBytes < 256 || v.MaxSQLBytes > 1<<20 || v.MaxOutputBytes < 1024 || v.MaxOutputBytes > 4<<20 {
		return invalid("pipelines", "invalid bounded runner limits")
	}
	if v.RunnerVersion != "v0.11.749" {
		return invalid("pipelines.runner_version", "unsupported artifact version")
	}
	if !v.Enabled {
		return nil
	}
	hash, err := hex.DecodeString(v.RunnerSHA256)
	if err != nil || len(hash) != 32 || hex.EncodeToString(hash) != v.RunnerSHA256 {
		return invalid("pipelines.runner_sha256", "exact lowercase artifact digest required")
	}
	if !filepath.IsAbs(v.RunnerPath) || !filepath.IsAbs(v.TempDir) {
		return invalid("pipelines", "absolute runner and private temporary directory required")
	}
	return nil
}
