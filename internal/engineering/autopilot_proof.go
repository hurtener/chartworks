package engineering

import (
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

func proposalAuthority(e identity.Envelope) string {
	return readexec.Hash([]any{e.Tenant(), e.User(), e.Session(), e.Scopes(), e.Reach()})
}

// PreparedProposal can only be issued after bounded inference/native validation
// in this package. A JSON request, model object or zero value is not this proof.
type PreparedProposal struct {
	body      []byte
	authority string
	deadline  time.Time
	limits    config.Autopilot
}

func prepareProposal(e identity.Envelope, m ProposalMaterial, l config.Autopilot) (PreparedProposal, error) {
	if m.Author != e.User() || m.Session != e.Session() {
		return PreparedProposal{}, ErrInvalid
	}
	if err := RequireAutopilotMaterial(e, m, "engineering.autopilot.propose"); err != nil {
		return PreparedProposal{}, err
	}
	if err := ValidateAutopilotMaterial(m, l); err != nil {
		return PreparedProposal{}, err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return PreparedProposal{}, ErrInvalid
	}
	return PreparedProposal{body: body, authority: proposalAuthority(e), deadline: e.Deadline(), limits: l}, nil
}

// Checked returns a detached exact proposal only to the original current caller.
func (p PreparedProposal) Checked(e identity.Envelope) (ProposalMaterial, error) {
	var m ProposalMaterial
	if !e.Valid() || p.authority == "" || p.authority != proposalAuthority(e) || !time.Now().Before(p.deadline) || len(p.body) == 0 || len(p.body) > 1<<20 || json.Unmarshal(p.body, &m) != nil {
		return m, ErrInvalid
	}
	if err := ValidateAutopilotMaterial(m, p.limits); err != nil {
		return ProposalMaterial{}, err
	}
	if m.Author != e.User() || m.Session != e.Session() {
		return ProposalMaterial{}, ErrInvalid
	}
	if err := RequireAutopilotMaterial(e, m, "engineering.autopilot.propose"); err != nil {
		return ProposalMaterial{}, err
	}
	return m, nil
}

// ProposalApply is addressed metadata, never a bearer or an executable SQL plan.
type ProposalApply struct {
	ID              string
	Revision        int64
	Digest          string
	ExpectedVersion int64
	Action          string
	Material        ProposalMaterial
	ExecutionDigest string
	Drift           *AutopilotDrift
}

// PreparedProposalApply binds reviewed material and the observed native effect
// evidence to this caller. It cannot widen ordinary pipeline publication scopes.
type PreparedProposalApply struct {
	body      []byte
	authority string
	deadline  time.Time
}

func prepareProposalApply(e identity.Envelope, p AutopilotProposal, action string, execution string, drift *AutopilotDrift) (PreparedProposalApply, error) {
	if p.ID == "" || p.Revision < 1 || p.Digest != p.Material.Digest() || p.Tenant != e.Tenant() {
		return PreparedProposalApply{}, ErrInvalid
	}
	if action != "engineering.autopilot.apply" && action != "engineering.autopilot.compensate" && action != "engineering.autopilot.drift" {
		return PreparedProposalApply{}, ErrInvalid
	}
	if err := RequireAutopilotMaterial(e, p.Material, action); err != nil {
		return PreparedProposalApply{}, err
	}
	if action != "engineering.autopilot.drift" {
		if p.Review == nil || p.Review.Decision != "approve" || p.Review.Digest != p.Digest || p.Review.Revision != p.Revision || p.Review.Actor == p.OriginAuthor || p.Review.Actor == p.Material.Author {
			return PreparedProposalApply{}, ErrProposalReview
		}
	}
	v := ProposalApply{ID: p.ID, Revision: p.Revision, Digest: p.Digest, ExpectedVersion: p.Version, Action: action, Material: p.Material, ExecutionDigest: execution, Drift: drift}
	body, err := json.Marshal(v)
	if err != nil {
		return PreparedProposalApply{}, ErrInvalid
	}
	return PreparedProposalApply{body: body, authority: proposalAuthority(e), deadline: e.Deadline()}, nil
}

// Checked cannot be replayed as another action, identity, session or proposal.
func (p PreparedProposalApply) Checked(e identity.Envelope, action string) (ProposalApply, error) {
	var v ProposalApply
	if !e.Valid() || p.authority == "" || p.authority != proposalAuthority(e) || !time.Now().Before(p.deadline) || len(p.body) == 0 || len(p.body) > 2<<20 || json.Unmarshal(p.body, &v) != nil {
		return v, ErrInvalid
	}
	if v.Action != action || v.ID != v.Material.Request.ID || v.Revision < 1 || v.Digest != v.Material.Digest() || v.ExpectedVersion < 1 {
		return ProposalApply{}, ErrInvalid
	}
	if err := RequireAutopilotMaterial(e, v.Material, action); err != nil {
		return ProposalApply{}, err
	}
	return v, nil
}
