package reporting

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// Publish changes only the active publication pointer. It does not certify,
// rewrite SQL, refresh validation, run a warehouse query or create an artifact.
func (s *Service) Publish(ctx context.Context, e identity.Envelope, id string, in PublishRequest) (State, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Publish)
	if err != nil {
		return State{}, err
	}
	defer cancel()
	if !identity.Identifier(in.Evidence) {
		return State{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Draft: true}, Publish)
	if err != nil {
		return State{}, err
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return State{}, err
	}
	if snapshot.State.Archived || snapshot.State.DraftState != "validated" || snapshot.PublishedAt != nil {
		return State{}, store.ErrConflict
	}
	if err := freshValidation(snapshot, in.Evidence, time.Now()); err != nil {
		return State{}, err
	}
	v := snapshot.Validation
	return s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "publish", ExpectedVersion: in.ExpectedVersion, TargetRevision: snapshot.Revision.Number, TargetDigest: snapshot.Revision.Digest, References: snapshotsReferences(snapshot), Evidence: in.Evidence, Watch: v.Dependencies, Topics: v.Topics, CheckCurrent: true})
}

// Certify records an immutable attestation for one exact published revision and
// validation receipt. It never confers source, SQL-read or publication authority.
func (s *Service) Certify(ctx context.Context, e identity.Envelope, id string, in CertifyRequest) (Attestation, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Certify)
	if err != nil {
		return Attestation{}, err
	}
	defer cancel()
	if in.Revision < 1 || !identity.Identifier(in.Evidence) || !note(in.Note) {
		return Attestation{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Revision: in.Revision}, Certify)
	if err != nil {
		return Attestation{}, err
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return Attestation{}, err
	}
	if snapshot.State.Archived || snapshot.PublishedAt == nil {
		return Attestation{}, store.ErrConflict
	}
	if err := freshValidation(snapshot, in.Evidence, time.Now()); err != nil {
		return Attestation{}, err
	}
	attestationID, err := newID()
	if err != nil {
		return Attestation{}, err
	}
	v := snapshot.Validation
	attestation := Attestation{ID: attestationID, Revision: in.Revision, Evidence: in.Evidence, Actor: e.User(), Note: in.Note, CreatedAt: time.Now().UTC(), EvidenceExpiresAt: v.Evidence.ExpiresAt, DependencyDigest: v.Evidence.DependencyDigest}
	_, err = s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "certify", ExpectedVersion: in.ExpectedVersion, TargetRevision: in.Revision, TargetDigest: snapshot.Revision.Digest, Note: in.Note, References: snapshotsReferences(snapshot), Evidence: in.Evidence, Attestation: &attestation, Watch: v.Dependencies, Topics: v.Topics, CheckCurrent: true})
	if err != nil {
		return Attestation{}, err
	}
	return clone(attestation), nil
}

// Withdraw appends a revocation of the selected attestation without destroying
// who certified what. Certification withdrawal is not publication rollback.
func (s *Service) Withdraw(ctx context.Context, e identity.Envelope, id string, in WithdrawRequest) (Withdrawal, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Certify)
	if err != nil {
		return Withdrawal{}, err
	}
	defer cancel()
	if !identity.Identifier(in.Attestation) || !note(in.Note) || in.Revision < 0 {
		return Withdrawal{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Revision: in.Revision}, Certify)
	if err != nil {
		return Withdrawal{}, err
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return Withdrawal{}, err
	}
	if snapshot.Attestation == nil || snapshot.Attestation.ID != in.Attestation || snapshot.Withdrawal != nil {
		return Withdrawal{}, store.ErrConflict
	}
	withdrawal := Withdrawal{Attestation: in.Attestation, Actor: e.User(), Note: in.Note, CreatedAt: time.Now().UTC()}
	_, err = s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "withdraw", ExpectedVersion: in.ExpectedVersion, TargetRevision: snapshot.Revision.Number, TargetDigest: snapshot.Revision.Digest, Note: in.Note, References: snapshotsReferences(snapshot), Withdrawal: &withdrawal})
	if err != nil {
		return Withdrawal{}, err
	}
	return withdrawal, nil
}

func (s *Service) Reject(ctx context.Context, e identity.Envelope, id string, in TransitionRequest) (State, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Write)
	if err != nil {
		return State{}, err
	}
	defer cancel()
	if !note(in.Note) {
		return State{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Draft: true}, Write)
	if err != nil {
		return State{}, err
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return State{}, err
	}
	if snapshot.State.Archived || snapshot.PublishedAt != nil || snapshot.State.DraftState == "rejected" {
		return State{}, store.ErrConflict
	}
	return s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "reject", ExpectedVersion: in.ExpectedVersion, TargetRevision: snapshot.Revision.Number, TargetDigest: snapshot.Revision.Digest, Note: in.Note, References: snapshotsReferences(snapshot)})
}

// Restore copies a retained authorized revision into a new private draft. Neither
// historic validation nor an attestation is transplanted to the new revision.
func (s *Service) Restore(ctx context.Context, e identity.Envelope, id string, in RestoreRequest) (View, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Write)
	if err != nil {
		return View{}, err
	}
	defer cancel()
	if in.Revision < 1 || !note(in.Note) {
		return View{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Revision: in.Revision}, Write)
	if err != nil {
		return View{}, err
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return View{}, err
	}
	provenance := clone(snapshot.Revision.Provenance)
	provenance.Kind, provenance.ParentRevision = "restore", in.Revision
	r, err := s.newRevision(e, snapshot.State.DraftRevision+1, snapshot.Revision.Definition, provenance)
	if err != nil {
		return View{}, err
	}
	state, err := s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "restore", ExpectedVersion: in.ExpectedVersion, TargetRevision: in.Revision, TargetDigest: snapshot.Revision.Digest, Note: in.Note, Revision: &r, References: snapshotsReferences(snapshot)})
	if err != nil {
		return View{}, err
	}
	return project(Snapshot{State: state, Revision: r}, time.Now()), nil
}

// Archive removes the default publication pointer but retains exact history.
// Restoring later creates an unvalidated draft, not an automatically active block.
func (s *Service) Archive(ctx context.Context, e identity.Envelope, id string, in TransitionRequest) (State, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Write)
	if err != nil {
		return State{}, err
	}
	defer cancel()
	if !note(in.Note) {
		return State{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Draft: true}, Write)
	if err != nil {
		return State{}, err
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return State{}, err
	}
	if snapshot.State.Archived {
		return State{}, store.ErrConflict
	}
	return s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "archive", ExpectedVersion: in.ExpectedVersion, TargetRevision: snapshot.Revision.Number, TargetDigest: snapshot.Revision.Digest, Note: in.Note, References: snapshotsReferences(snapshot)})
}
