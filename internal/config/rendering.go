package config

import "time"

// Rendering bounds the feature-gated isolated static worker and retained renditions.
// WorkerPath is an operator supplied executable path, never a URL or shell fragment.
type Rendering struct {
	Enabled        bool     `json:"enabled"`
	WorkerPath     string   `json:"worker_path"`
	WorkerVersion  string   `json:"worker_version"`
	ThemeVersion   string   `json:"theme_version"`
	MaxTime        Duration `json:"max_time"`
	MaxMemoryBytes int64    `json:"max_memory_bytes"`
	MaxInputBytes  int      `json:"max_input_bytes"`
	MaxOutputBytes int      `json:"max_output_bytes"`
	MaxConcurrent  int      `json:"max_concurrent"`
	MaxWidgets     int      `json:"max_widgets"`
	Retention      Duration `json:"retention"`
	Isolation      string   `json:"isolation"`
}

// DefaultRendering returns bounded disabled-renderer defaults.
func DefaultRendering() Rendering {
	return Rendering{WorkerVersion: "chartworks-svg-worker-v1", ThemeVersion: "chartworks-theme-v1", MaxTime: Duration(10 * time.Second), MaxMemoryBytes: 1 << 30, MaxInputBytes: 16 << 20, MaxOutputBytes: 16 << 20, MaxConcurrent: 2, MaxWidgets: 100, Retention: Duration(24 * time.Hour), Isolation: "linux_namespaces"}
}

// Validate rejects open process, resource and retention configuration.
func (c Rendering) Validate() error {
	if c.Enabled && (c.WorkerPath == "" || c.WorkerPath[0] != '/' || len(c.WorkerPath) > 4096) ||
		c.WorkerVersion == "" || len(c.WorkerVersion) > 128 || c.ThemeVersion == "" || len(c.ThemeVersion) > 128 ||
		time.Duration(c.MaxTime) < 100*time.Millisecond || time.Duration(c.MaxTime) > time.Minute ||
		c.MaxMemoryBytes < 32<<20 || c.MaxMemoryBytes > 2<<30 || c.MaxInputBytes < 1024 || c.MaxInputBytes > 64<<20 ||
		c.MaxOutputBytes < 1024 || c.MaxOutputBytes > 64<<20 || c.MaxConcurrent < 1 || c.MaxConcurrent > 16 || c.MaxWidgets < 1 || c.MaxWidgets > 1000 ||
		time.Duration(c.Retention) < time.Minute || time.Duration(c.Retention) > 90*24*time.Hour || c.Isolation != "linux_namespaces" {
		return invalid("rendering", "bounded local worker, retention and parent policy required")
	}
	return nil
}

func (c Rendering) validate() error { return c.Validate() }
