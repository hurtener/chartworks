package migration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// EvaluationAdapters imports Phase 24 material only through the authority-bound
// evaluation service. Both objects remain drafts and require later independent
// review; historical run evidence remains quarantined as KindRun.
func EvaluationAdapters(service *evaluation.Service) map[Kind]Adapter {
	if service == nil {
		return nil
	}
	return map[Kind]Adapter{
		KindCalibration: AdapterFuncs{
			ValidateFunc: func(ctx context.Context, envelope identity.Envelope, object Object, _ Mapping) error {
				var in evaluation.ProposalRequest
				if err := decodeEvaluationPayload(object.Payload, &in); err != nil {
					return err
				}
				_, err := service.PreviewOptimization(ctx, envelope, in)
				return err
			},
			ApplyFunc: func(ctx context.Context, envelope identity.Envelope, object Object, _ Mapping, _ string) (string, error) {
				var in evaluation.ProposalRequest
				if err := decodeEvaluationPayload(object.Payload, &in); err != nil {
					return "", err
				}
				proposal, err := service.ProposeOptimization(ctx, envelope, in)
				if errors.Is(err, store.ErrConflict) {
					proposal, err = service.DraftOptimization(ctx, envelope, in.ID)
					if err == nil && (proposal.SuiteID != in.SuiteID || proposal.SuiteRevision != in.SuiteRevision || proposal.SuiteDigest != in.SuiteDigest || proposal.Baseline.ID != in.BaselineRun || proposal.Candidate.ID != in.CandidateRun) {
						err = store.ErrConflict
					}
				}
				if err != nil {
					return "", err
				}
				return "optimization:" + proposal.ID + ":candidate", nil
			},
		},
		KindRuntimePack: AdapterFuncs{
			ValidateFunc: func(_ context.Context, _ identity.Envelope, object Object, _ Mapping) error {
				var in evaluation.RuntimePackAuthorRequest
				if err := decodeEvaluationPayload(object.Payload, &in); err != nil {
					return err
				}
				return in.Validate()
			},
			ApplyFunc: func(ctx context.Context, envelope identity.Envelope, object Object, _ Mapping, _ string) (string, error) {
				var in evaluation.RuntimePackAuthorRequest
				if err := decodeEvaluationPayload(object.Payload, &in); err != nil {
					return "", err
				}
				record, err := service.AuthorRuntimePack(ctx, envelope, in.Pack, in.Config)
				if errors.Is(err, store.ErrConflict) {
					record, err = service.DraftRuntimePack(ctx, envelope, in.Pack.Digest)
					if err == nil && (!reflect.DeepEqual(record.Pack, in.Pack) || !reflect.DeepEqual(record.Config, in.Config)) {
						err = store.ErrConflict
					}
				}
				if err != nil {
					return "", err
				}
				return "runtime-pack:" + record.Pack.Digest + ":draft", nil
			},
		},
		KindEvalSuite: AdapterFuncs{
			ValidateFunc: func(_ context.Context, _ identity.Envelope, object Object, _ Mapping) error {
				var suite evaluation.Suite
				if err := decodeEvaluationPayload(object.Payload, &suite); err != nil {
					return err
				}
				return suite.Validate()
			},
			ApplyFunc: func(ctx context.Context, envelope identity.Envelope, object Object, _ Mapping, _ string) (string, error) {
				var suite evaluation.Suite
				if err := decodeEvaluationPayload(object.Payload, &suite); err != nil {
					return "", err
				}
				record, err := service.Author(ctx, envelope, suite)
				if errors.Is(err, store.ErrConflict) {
					record, err = service.DraftSuite(ctx, envelope, suite.ID, suite.Revision)
					want, digestErr := suite.Digest()
					if err == nil && (digestErr != nil || record.Digest != want || !reflect.DeepEqual(record.Suite, suite)) {
						err = store.ErrConflict
					}
				}
				if err != nil {
					return "", err
				}
				return "evaluation-suite:" + record.Suite.ID + ":" + strconv.FormatInt(record.Suite.Revision, 10) + ":draft", nil
			},
		},
	}
}

func decodeEvaluationPayload(raw string, out any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func validateEvaluationObject(object Object) error {
	switch object.Kind {
	case KindRuntimePack:
		var in evaluation.RuntimePackAuthorRequest
		if err := decodeEvaluationPayload(object.Payload, &in); err != nil {
			return err
		}
		return in.Validate()
	case KindEvalSuite:
		var suite evaluation.Suite
		if err := decodeEvaluationPayload(object.Payload, &suite); err != nil {
			return err
		}
		return suite.Validate()
	case KindCalibration:
		var in evaluation.ProposalRequest
		if err := decodeEvaluationPayload(object.Payload, &in); err != nil {
			return err
		}
		if !identity.Identifier(in.ID) || !identity.Identifier(in.SuiteID) || in.SuiteRevision < 1 || !validHash(in.SuiteDigest) || !identity.Identifier(in.BaselineRun) || !identity.Identifier(in.CandidateRun) || in.BaselineRun == in.CandidateRun {
			return ErrInvalid
		}
		return nil
	default:
		return nil
	}
}

func evaluationObjectDigests(objects []Object) (runtimePacks, suites map[string]bool, err error) {
	runtimePacks, suites = map[string]bool{}, map[string]bool{}
	for _, object := range objects {
		switch object.Kind {
		case KindRuntimePack:
			var in evaluation.RuntimePackAuthorRequest
			if err = decodeEvaluationPayload(object.Payload, &in); err != nil {
				return nil, nil, err
			}
			var digest string
			digest, err = in.Digest()
			if err != nil {
				return nil, nil, ErrInvalid
			}
			runtimePacks[digest] = true
		case KindEvalSuite:
			var suite evaluation.Suite
			if err = decodeEvaluationPayload(object.Payload, &suite); err != nil {
				return nil, nil, err
			}
			var digest string
			digest, err = suite.Digest()
			if err != nil {
				return nil, nil, ErrInvalid
			}
			suites[digest] = true
		}
	}
	return runtimePacks, suites, nil
}
