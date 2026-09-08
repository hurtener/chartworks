package sourceapi

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/jobs"
)

const pipelineRequestMaxBytes = 1 << 20

type pipelinePublishRequest struct {
	Version int64 `json:"version"`
}

func pipelineErrors() []api.ErrorResponse {
	return append(engineeringErrors(), api.ErrorResponse{422, "pipeline_quality_failed"})
}

// PipelineAPIRegistry supplies the existing handler, manifest and OpenAPI from one inventory.
func PipelineAPIRegistry(enabled bool) (*api.Registry, error) {
	definitions := []registration{
		{Operation{"POST", "/v1/pipeline-versions/read", "engineering.pipeline.read", "private_definition_read"}, "readPipelineVersion", "Read an immutable private pipeline version", "engineering.PipelineService.Get", "read_only_no_domain_audit", reflect.TypeFor[pipelineVersionRequest](), reflect.TypeFor[engineering.PipelineVersion]()},
		{Operation{"GET", "/v1/pipeline-runs/{id}", "jobs.read", "private_run_metadata_read"}, "readPipelineRun", "Read retained private pipeline run", "engineering.PipelineService.InspectRun", "read_only_no_domain_audit", nil, reflect.TypeFor[engineering.PipelineRun]()},
		{Operation{"POST", "/v1/pipeline-runs/{id}/cancel", "jobs.cancel", "durable_cancellation_intent"}, "cancelPipelineRun", "Record private pipeline cancellation", "engineering.PipelineService.Cancel", "request.cancelled", reflect.TypeFor[struct{}](), reflect.TypeFor[jobs.RequestTask]()},
	}
	if enabled {
		definitions = append(definitions,
			registration{Operation{"POST", "/v1/pipeline-proposals", "engineering.pipeline.write", "model_assisted_pipeline_draft"}, "proposePipeline", "Propose a bounded unapproved pipeline draft", "engineering.PipelineService.Propose", "pipeline.drafted", reflect.TypeFor[engineering.PipelineProposalRequest](), reflect.TypeFor[engineering.PipelineVersion]()},
			registration{Operation{"POST", "/v1/pipelines", "engineering.pipeline.write", "versioned_pipeline_draft"}, "draftPipeline", "Store a versioned private pipeline draft", "engineering.PipelineService.Draft", "pipeline.drafted", reflect.TypeFor[pipelineDraftRequest](), reflect.TypeFor[engineering.PipelineVersion]()},
			registration{Operation{"POST", "/v1/pipelines/{id}/publish", "engineering.pipeline.publish", "immutable_pipeline_publication"}, "publishPipeline", "Publish the exact validated pipeline version", "engineering.PipelineService.Publish", "pipeline.published", reflect.TypeFor[pipelinePublishRequest](), reflect.TypeFor[engineering.PipelineVersion]()},
			registration{Operation{"POST", "/v1/pipelines/{id}/runs", "engineering.pipeline.run", "managed_warehouse_write"}, "runPipeline", "Admit or execute one managed pipeline run", "engineering.PipelineService.AdmitRun; engineering.PipelineService.Run", "request journal; pipeline effects and activation", reflect.TypeFor[pipelineRunRequest](), reflect.TypeFor[engineering.PipelineRun]()},
		)
	}
	return compileRegistrations(definitions, pipelineErrors(), pipelineRequestMaxBytes, true, 0)
}
