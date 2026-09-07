package chartworks

import (
	"context"
	"errors"
	"time"
)

type PipelineColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey bool   `json:"primary_key"`
}
type PipelineCheck struct {
	Kind    string `json:"kind"`
	Column  string `json:"column,omitempty"`
	Minimum int64  `json:"minimum,omitempty"`
	Maximum int64  `json:"maximum,omitempty"`
}
type PipelineStep struct {
	ID         string           `json:"id"`
	Source     string           `json:"source,omitempty"`
	Context    string           `json:"context,omitempty"`
	SQL        string           `json:"sql"`
	Inputs     []string         `json:"inputs"`
	DependsOn  []string         `json:"depends_on"`
	FromSteps  []string         `json:"from_steps,omitempty"`
	Strategy   string           `json:"strategy"`
	Columns    []PipelineColumn `json:"columns"`
	Key        string           `json:"key,omitempty"`
	TimeColumn string           `json:"time_column,omitempty"`
	Start      string           `json:"start,omitempty"`
	End        string           `json:"end,omitempty"`
	Checks     []PipelineCheck  `json:"checks"`
}
type PipelineDefinition struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Connection string         `json:"connection"`
	Steps      []PipelineStep `json:"steps"`
}
type PipelineProposalRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Connection  string `json:"connection"`
	Source      string `json:"source"`
	Context     string `json:"context"`
	Instruction string `json:"instruction"`
}
type PipelineVersion struct {
	Definition PipelineDefinition `json:"definition"`
	Version    int64              `json:"version"`
	State      string             `json:"state"`
	Digest     string             `json:"digest"`
	Created    time.Time          `json:"created_at"`
	Published  *time.Time         `json:"published_at,omitempty"`
}
type PipelineEffect struct {
	Step    string `json:"step"`
	State   string `json:"state"`
	Rows    int64  `json:"rows"`
	Source  string `json:"source,omitempty"`
	Context string `json:"context,omitempty"`
	Digest  string `json:"digest"`
	Code    string `json:"code,omitempty"`
}
type PipelineRun struct {
	Pipeline  string               `json:"pipeline"`
	Version   int64                `json:"version"`
	State     string               `json:"state"`
	Effects   []PipelineEffect     `json:"effects"`
	Operation EngineeringOperation `json:"operation"`
}

// ProposePipeline asks the configured model gateway for a validated draft. It
// cannot publish or run the resulting immutable version.
func (c *Client) ProposePipeline(ctx context.Context, request PipelineProposalRequest) (out PipelineVersion, err error) {
	if !wireID(request.ID) || !wireID(request.Connection) || !wireID(request.Source) || !wireID(request.Context) || request.Name == "" || request.Instruction == "" {
		return out, errors.New("chartworks: invalid pipeline proposal")
	}
	err = c.call(ctx, "POST", "/v1/pipeline-proposals", "", request, &out)
	return out, err
}

// DraftPipeline creates the next immutable draft under explicit CAS.
func (c *Client) DraftPipeline(ctx context.Context, definition PipelineDefinition, expected int64) (out PipelineVersion, err error) {
	if !wireID(definition.ID) || !wireID(definition.Connection) || expected < 0 || len(definition.Steps) < 1 {
		return out, errors.New("chartworks: invalid pipeline draft")
	}
	for i := range definition.Steps {
		if definition.Steps[i].Inputs == nil {
			definition.Steps[i].Inputs = []string{}
		}
		if definition.Steps[i].DependsOn == nil {
			definition.Steps[i].DependsOn = []string{}
		}
		if definition.Steps[i].FromSteps == nil {
			definition.Steps[i].FromSteps = []string{}
		}
		if definition.Steps[i].Columns == nil {
			definition.Steps[i].Columns = []PipelineColumn{}
		}
		if definition.Steps[i].Checks == nil {
			definition.Steps[i].Checks = []PipelineCheck{}
		}
	}
	body := struct {
		Definition PipelineDefinition `json:"definition"`
		Expected   int64              `json:"expected_revision"`
	}{definition, expected}
	err = c.call(ctx, "POST", "/v1/pipelines", "", body, &out)
	return out, err
}

func (c *Client) PublishPipeline(ctx context.Context, id string, version int64) (out PipelineVersion, err error) {
	if !wireID(id) || version < 1 {
		return out, errors.New("chartworks: invalid pipeline version")
	}
	err = c.call(ctx, "POST", "/v1/pipelines/"+id+"/publish", "", struct {
		Version int64 `json:"version"`
	}{version}, &out)
	return out, err
}

// Pipeline reads one exact immutable version; there is no implicit latest lookup.
func (c *Client) Pipeline(ctx context.Context, id string, version int64) (out PipelineVersion, err error) {
	if !wireID(id) || version < 1 {
		return out, errors.New("chartworks: invalid pipeline version")
	}
	body := struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}{id, version}
	err = c.call(ctx, "POST", "/v1/pipeline-versions/read", "", body, &out)
	return out, err
}

// RunPipeline starts or explicitly resumes one published immutable version.
func (c *Client) RunPipeline(ctx context.Context, id string, version int64, key string, resume bool) (out PipelineRun, err error) {
	if !wireID(id) || version < 1 || !wireID(key) {
		return out, errors.New("chartworks: invalid pipeline run")
	}
	body := struct {
		Version   int64  `json:"version"`
		Key       string `json:"key"`
		Resume    bool   `json:"resume"`
		AdmitOnly bool   `json:"admit_only"`
	}{version, key, resume, false}
	err = c.call(ctx, "POST", "/v1/pipelines/"+id+"/runs", "", body, &out)
	return out, err
}

// AdmitPipelineRun reserves a durable operation without starting warehouse
// work, allowing an authorized caller to inspect or cancel before dispatch.
func (c *Client) AdmitPipelineRun(ctx context.Context, id string, version int64, key string) (out PipelineRun, err error) {
	if !wireID(id) || version < 1 || !wireID(key) {
		return out, errors.New("chartworks: invalid pipeline run")
	}
	body := struct {
		Version   int64  `json:"version"`
		Key       string `json:"key"`
		Resume    bool   `json:"resume"`
		AdmitOnly bool   `json:"admit_only"`
	}{version, key, false, true}
	err = c.call(ctx, "POST", "/v1/pipelines/"+id+"/runs", "", body, &out)
	return out, err
}

func (c *Client) PipelineRun(ctx context.Context, operation string) (out PipelineRun, err error) {
	if !wireID(operation) {
		return out, errors.New("chartworks: invalid pipeline operation")
	}
	err = c.call(ctx, "GET", "/v1/pipeline-runs/"+operation, "", nil, &out)
	return out, err
}

// CancelPipelineRun records intent; the receipt does not claim the external process stopped.
func (c *Client) CancelPipelineRun(ctx context.Context, operation string) (out EngineeringOperation, err error) {
	if !wireID(operation) {
		return out, errors.New("chartworks: invalid pipeline operation")
	}
	err = c.call(ctx, "POST", "/v1/pipeline-runs/"+operation+"/cancel", "", struct{}{}, &out)
	return out, err
}
