package sourceapi

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/jobs"
)

type uploadSweepRequest struct {
	Limit int `json:"limit"`
}
type dependencyRegistered struct {
	Registered bool `json:"registered"`
}

func engineeringErrors() []api.ErrorResponse {
	return append(sourceErrors(), api.ErrorResponse{422, "unsafe_or_unsupported_file"}, api.ErrorResponse{422, "checksum_mismatch"}, api.ErrorResponse{409, "workspace_ownership_unproven"}, api.ErrorResponse{409, "reconciliation_required"}, api.ErrorResponse{403, "authority_blocked"}, api.ErrorResponse{410, "expired"})
}

// EngineeringAPIRegistry supplies the existing handler, manifest and OpenAPI from one inventory.
func EngineeringAPIRegistry(uploads, profiles bool, uploadBytes int64) (*api.Registry, error) {
	if uploads && (uploadBytes < 1 || uploadBytes > 100<<20) {
		return nil, api.ErrRegistration
	}
	definitions := []registration{
		{Operation{"GET", "/v1/uploads/{id}", "sources.read", "private_metadata_read"}, "readUpload", "Read private upload status", "engineering.Service.InspectUpload", "read_only_no_domain_audit", nil, reflect.TypeFor[engineering.UploadStatus]()},
		{Operation{"GET", "/v1/profiles/{id}", "engineering.read", "private_metadata_read"}, "readProfile", "Read private profile status", "engineering.Service.InspectProfile", "read_only_no_domain_audit", nil, reflect.TypeFor[engineering.ProfileStatus]()},
		{Operation{"GET", "/v1/profiles/{id}/evidence", "engineering.read", "private_evidence_read"}, "profileEvidence", "Read retained private profile evidence", "engineering.Service.Evidence", "read_only_no_domain_audit", nil, reflect.TypeFor[engineering.ProfileEvidence]()},
		{Operation{"POST", "/v1/profile-history", "engineering.read", "private_metadata_read"}, "profileHistory", "Read private profile history", "engineering.Service.History", "read_only_no_domain_audit", reflect.TypeFor[ProfileHistoryRequest](), reflect.TypeFor[[]engineering.ProfileStatus]()},
		{Operation{"POST", "/v1/profile-dependency-health", "engineering.read", "private_metadata_read"}, "profileHealth", "Read exact dependency health", "engineering.Service.Health", "read_only_no_domain_audit", reflect.TypeFor[engineering.Dependency](), reflect.TypeFor[[]engineering.HealthEvent]()},
		{Operation{"GET", "/v1/engineering-operations/{id}", "jobs.read", "private_metadata_read"}, "engineeringOperation", "Read private engineering operation", "engineering.Service.RequestOperation", "read_only_no_domain_audit", nil, reflect.TypeFor[jobs.RequestTask]()},
		{Operation{"POST", "/v1/engineering-operations/{id}/cancel", "jobs.cancel", "durable_cancellation_intent"}, "cancelEngineering", "Record private operation cancellation", "engineering.Service.CancelOperation", "request.cancelled", reflect.TypeFor[struct{}](), reflect.TypeFor[jobs.RequestTask]()},
	}
	if uploads {
		definitions = append(definitions,
			registration{Operation{"POST", "/v1/uploads", "sources.upload", "upload_reservation"}, "reserveUpload", "Reserve a bounded private upload", "engineering.Service.ReserveUpload", "upload.reserved", reflect.TypeFor[engineering.UploadSpec](), reflect.TypeFor[engineering.UploadStatus]()},
			registration{Operation{"PUT", "/v1/uploads/{id}/content", "sources.upload", "workspace_staging_write"}, "stageUpload", "Stage reserved binary upload content", "engineering.Service.StageUpload", "upload.staged", reflect.TypeFor[uploadContent](), reflect.TypeFor[engineering.UploadStatus]()},
			registration{Operation{"POST", "/v1/uploads/{id}/load", "sources.upload", "managed_source_activation"}, "loadUpload", "Load and activate an owned workspace upload", "engineering.Service.LoadUpload", "request journal; upload.activated", reflect.TypeFor[UploadAction](), reflect.TypeFor[engineering.UploadRun]()},
			registration{Operation{"POST", "/v1/uploads/{id}/erase", "sources.erase", "owned_workspace_erasure"}, "eraseUpload", "Erase owned workspace upload and evidence", "engineering.Service.EraseUpload", "request journal; upload.erased", reflect.TypeFor[UploadAction](), reflect.TypeFor[engineering.UploadRun]()},
			registration{Operation{"POST", "/v1/upload-sweeps", "sources.erase", "expired_staging_erasure"}, "sweepUploads", "Erase expired private staging uploads", "engineering.Service.SweepUploads", "request journal; upload.erased", reflect.TypeFor[uploadSweepRequest](), reflect.TypeFor[[]engineering.UploadRun]()},
		)
	}
	if profiles {
		definitions = append(definitions,
			registration{Operation{"POST", "/v1/profiles", "engineering.profile", "bounded_source_profile_and_optional_model"}, "buildProfile", "Build bounded private profile evidence", "engineering.Service.Build", "request journal; profile checkpoint and publication", reflect.TypeFor[ProfileRequest](), reflect.TypeFor[engineering.ProfileRun]()},
			registration{Operation{"POST", "/v1/profiles/{id}/dependencies", "engineering.profile", "versioned_dependency_registration"}, "profileDependency", "Register an exact profile dependency", "engineering.Service.RegisterDependency", "profile.dependency_registered", reflect.TypeFor[engineering.Dependency](), reflect.TypeFor[dependencyRegistered]()},
		)
	}
	return compileRegistrations(definitions, engineeringErrors(), sourceRequestMaxBytes, false, uploadBytes)
}
