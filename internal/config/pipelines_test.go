package config

import (
	"strings"
	"testing"
	"time"
)

func TestPipelinesBoundedArtifactConfiguration(t *testing.T) {
	base := DefaultPipelines()
	if err := ValidatePipelines(base); err != nil {
		t.Fatal(err)
	}
	base.Enabled = true
	base.RunnerPath = "/opt/chartworks/bruin"
	base.TempDir = "/runtime"
	base.RunnerSHA256 = strings.Repeat("a", 64)
	if err := ValidatePipelines(base); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Pipelines){
		"short timeout": func(v *Pipelines) { v.Timeout = Duration(time.Millisecond) }, "long timeout": func(v *Pipelines) { v.Timeout = Duration(2 * time.Minute) },
		"concurrency zero": func(v *Pipelines) { v.Concurrency = 0 }, "concurrency excess": func(v *Pipelines) { v.Concurrency = 9 },
		"steps zero": func(v *Pipelines) { v.MaxSteps = 0 }, "steps excess": func(v *Pipelines) { v.MaxSteps = 33 },
		"sql small": func(v *Pipelines) { v.MaxSQLBytes = 1 }, "sql large": func(v *Pipelines) { v.MaxSQLBytes = 2 << 20 },
		"output small": func(v *Pipelines) { v.MaxOutputBytes = 1 }, "output large": func(v *Pipelines) { v.MaxOutputBytes = 8 << 20 },
		"version": func(v *Pipelines) { v.RunnerVersion = "latest" }, "digest missing": func(v *Pipelines) { v.RunnerSHA256 = "" }, "digest uppercase": func(v *Pipelines) { v.RunnerSHA256 = strings.Repeat("A", 64) },
		"relative executable": func(v *Pipelines) { v.RunnerPath = "bruin" }, "relative scratch": func(v *Pipelines) { v.TempDir = "tmp" },
	} {
		t.Run(name, func(t *testing.T) {
			v := base
			change(&v)
			if ValidatePipelines(v) == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}
