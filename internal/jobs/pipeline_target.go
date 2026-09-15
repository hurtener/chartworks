package jobs

import (
	"encoding/hex"

	"github.com/hurtener/chartworks/internal/identity"
)

// PipelineKind identifies execution of an exact published managed pipeline.
const PipelineKind = "pipeline.run"

// PipelineTarget pins the published version accepted by the schedule. A new
// publication never silently changes the meaning of an existing occurrence.
type PipelineTarget struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Digest  string `json:"digest"`
}

// Valid rejects unversioned, malformed or noncanonical targets.
func (p PipelineTarget) Valid() bool {
	b, err := hex.DecodeString(p.Digest)
	return identity.Identifier(p.ID) && len(p.ID) <= 48 && p.Version > 0 && p.Version < 1<<62 && err == nil && len(b) == 32 && hex.EncodeToString(b) == p.Digest
}
