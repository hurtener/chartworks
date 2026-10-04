package reporting

import (
	"context"
	"strconv"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// AuthoringBlockReadRequest always selects an immutable exact revision. It does
// not infer a draft/publication pointer or admit a client-owned definition.
type AuthoringBlockReadRequest struct {
	Block    string `json:"block"`
	Revision int64  `json:"revision" jsonschema:"minimum=1,maximum=256"`
}

// AuthoringOutputColumns supplies server-owned binding candidates for one
// selected output. Existing column IDs and reviewed metadata remain unchanged.
type AuthoringOutputColumns struct {
	Output  string          `json:"output"`
	Columns []charts.Column `json:"columns"`
}

// AuthoringBlockView is SQL-free, like the native block View. Mapping checks
// against an expected schema are never represented as observed-data validation.
type AuthoringBlockView struct {
	Block          AuthoringBlockMetadata   `json:"block"`
	OutputColumns  []AuthoringOutputColumns `json:"output_columns"`
	DataValidation string                   `json:"data_validation" jsonschema:"enum=not_performed"`
}

// AuthoringBlockOutput deliberately has no narrative instructions or policies.
// Noneditable outputs retain stable identity/intent without exposing a prompt.
type AuthoringBlockOutput struct {
	ID                 string                           `json:"id"`
	Kind               string                           `json:"kind"`
	Intent             *OutputIntent                    `json:"intent,omitempty"`
	Mapping            *charts.Mapping                  `json:"mapping,omitempty"`
	AmountCompleteness []AmountOutputBinding            `json:"amount_completeness,omitempty"`
	Editable           bool                             `json:"editable"`
	Presentation       *charts.PresentationCapabilities `json:"presentation,omitempty"`
}

// AuthoringBlockValidation exposes only revision-bound evidence coordinates;
// query attempts, native remote identifiers and session provenance stay private.
type AuthoringBlockValidation struct {
	ID               string       `json:"id"`
	Revision         int64        `json:"revision"`
	DefinitionDigest string       `json:"definition_digest"`
	ExecutionDigest  string       `json:"execution_digest"`
	SchemaDigest     string       `json:"schema_digest"`
	Schema           []exec.Field `json:"schema"`
	CreatedAt        time.Time    `json:"created_at"`
	ExpiresAt        time.Time    `json:"expires_at"`
}

// AuthoringBlockMetadata is an explicit safe projection, not an embedded View
// that could acquire new private output payloads through future native changes.
type AuthoringBlockMetadata struct {
	AmountCompleteness []AmountDeclaration       `json:"amount_completeness,omitempty"`
	SchemaVersion      int                       `json:"schema_version"`
	QueryLimits        *QueryLimits              `json:"query_limits,omitempty"`
	ResultPolicy       []EffectiveFieldPolicy    `json:"result_policy"`
	State              State                     `json:"state"`
	Revision           int64                     `json:"revision"`
	RevisionID         string                    `json:"revision_id"`
	Digest             string                    `json:"digest"`
	ExecutionDigest    string                    `json:"execution_digest"`
	Metadata           []Localized               `json:"metadata"`
	Source             string                    `json:"source"`
	Context            string                    `json:"context"`
	Topics             []TopicPin                `json:"topics"`
	Rules              []RulePin                 `json:"rules,omitempty"`
	Parameters         []Parameter               `json:"parameters"`
	ExpectedSchema     []exec.Field              `json:"expected_schema"`
	Outputs            []AuthoringBlockOutput    `json:"outputs"`
	Actor              string                    `json:"actor"`
	CreatedAt          time.Time                 `json:"created_at"`
	Private            bool                      `json:"private"`
	Trust              Trust                     `json:"trust"`
	Evidence           *AuthoringBlockValidation `json:"validation,omitempty"`
}

// AuthoringChartMapping is deliberately not charts.Mapping: callers cannot
// inject result rows, columns, types, semantic provenance, SQL or a definition.
// An absent intent preserves the exact stored intent. Legacy v1 intent changes
// require an explicit native migration, never changes to untargeted outputs.
type AuthoringChartMapping struct {
	Kind     charts.Kind          `json:"kind" jsonschema:"enum=area,enum=bar,enum=column,enum=donut,enum=grouped_bar,enum=heatmap,enum=kpi,enum=line,enum=pie,enum=scatter,enum=stacked_bar,enum=table,enum=treemap,enum=stacked_column"`
	Bindings charts.Bindings      `json:"bindings"`
	Order    []charts.Order       `json:"order"`
	Options  charts.Options       `json:"options"`
	KPI      *charts.KPIOptions   `json:"kpi,omitempty"`
	Table    *charts.TableOptions `json:"table,omitempty"`
	Intent   *OutputIntent        `json:"intent,omitempty"`
}

type AuthoringBlockMappingRequest struct {
	Block           string                `json:"block"`
	ExpectedVersion int64                 `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64                 `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string                `json:"digest"`
	Output          string                `json:"output"`
	Mapping         AuthoringChartMapping `json:"mapping"`
}

// AuthoringBlockCopyRequest names a distinct, already authorized new block. It
// creates only a private unvalidated copy; no report or source block is changed.
type AuthoringBlockCopyRequest struct {
	Block           string                `json:"block"`
	ExpectedVersion int64                 `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64                 `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string                `json:"digest"`
	Output          string                `json:"output"`
	NewBlock        string                `json:"new_block"`
	Mapping         AuthoringChartMapping `json:"mapping"`
}

// AuthoringBlockCopyRepository is an internal full-definition custody seam.
// It is not a client SQL projection. Implementations must enforce source read
// and preview, tenant/dependency eligibility and private ownership before loading
// the exact immutable source; ordinary ReadBlock(Read) remains SQL-redacted.
type AuthoringBlockCopyRepository interface {
	ReadBlockForAuthoringCopy(context.Context, identity.Envelope, string, Reference) (Snapshot, error)
}

func (s *Authoring) blockService() (*Service, error) {
	if s == nil || s.documents == nil || s.documents.blocks == nil {
		return nil, ErrUnavailable
	}
	return s.documents.blocks, nil
}

func authoringBlockView(v View, limits config.Reporting) AuthoringBlockView {
	v = clone(v)
	metadata := AuthoringBlockMetadata{AmountCompleteness: v.AmountCompleteness, SchemaVersion: v.SchemaVersion, QueryLimits: v.QueryLimits, ResultPolicy: v.ResultPolicy, State: v.State, Revision: v.Revision, RevisionID: v.RevisionID, Digest: v.Digest, ExecutionDigest: v.ExecutionDigest, Metadata: v.Metadata, Source: v.Source, Context: v.Context, Topics: v.Topics, Rules: v.Rules, Parameters: v.Parameters, ExpectedSchema: v.ExpectedSchema, Outputs: []AuthoringBlockOutput{}, Actor: v.Actor, CreatedAt: v.CreatedAt, Private: v.Private, Trust: v.Trust}
	if evidence := v.Evidence; evidence != nil {
		safe := authoringBlockEvidence(*evidence)
		metadata.Evidence = &safe
	}
	out := AuthoringBlockView{Block: metadata, OutputColumns: []AuthoringOutputColumns{}, DataValidation: "not_performed"}
	for _, output := range v.Outputs {
		editable := output.Mapping != nil && output.Narrative == nil && output.Kind != "narrative"
		item := AuthoringBlockOutput{ID: output.ID, Kind: output.Kind, Intent: output.Intent, AmountCompleteness: output.AmountCompleteness, Editable: editable}
		if editable {
			item.Mapping = output.Mapping
			if capabilities, err := charts.PresentationCapabilitiesFor(context.Background(), *output.Mapping, chartLimits(limits)); err == nil {
				item.Presentation = &capabilities
			}
		}
		out.Block.Outputs = append(out.Block.Outputs, item)
		if output.Mapping != nil && output.Narrative == nil {
			out.OutputColumns = append(out.OutputColumns, AuthoringOutputColumns{Output: output.ID, Columns: authoringColumns(v.ExpectedSchema, *output.Mapping)})
		}
	}
	return out
}

// authoringColumns preserves all reviewed selected-output metadata. Previously
// unbound expected fields get inert, collision-free IDs and no invented measure,
// aggregation, unit or semantic approval. Actual data still needs validation.
func authoringColumns(schema []exec.Field, mapping charts.Mapping) []charts.Column {
	out := clone(mapping.Columns)
	ids, names := map[string]bool{}, map[string]bool{}
	for _, c := range out {
		ids[c.ID], names[c.Name] = true, true
	}
	for i, f := range schema {
		if names[f.Name] {
			continue
		}
		id := "field_" + strconv.Itoa(i)
		for suffix := 1; ids[id]; suffix++ {
			id = "field_" + strconv.Itoa(i) + "_" + strconv.Itoa(suffix)
		}
		role := "unknown"
		kind := chartType(f.Type)
		out = append(out, charts.Column{ID: id, Name: f.Name, Type: kind, Role: role, Provenance: charts.Provenance{Version: 1}})
		ids[id], names[f.Name] = true, true
	}
	return out
}

func (s *Authoring) ReadBlock(ctx context.Context, e identity.Envelope, in AuthoringBlockReadRequest) (AuthoringBlockView, error) {
	if s == nil || ctx == nil || !identity.Identifier(in.Block) || in.Revision < 1 || in.Revision > 256 {
		return AuthoringBlockView{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return AuthoringBlockView{}, err
	}
	if err := Require(e, in.Block, Read); err != nil {
		return AuthoringBlockView{}, err
	}
	blocks, err := s.blockService()
	if err != nil {
		return AuthoringBlockView{}, err
	}
	v, err := blocks.Read(ctx, e, in.Block, Reference{Revision: in.Revision})
	if err != nil {
		return AuthoringBlockView{}, err
	}
	return authoringBlockView(v, blocks.limits), nil
}

func authoringMappingInput(block string, version, revision int64, digest, output string) error {
	if !identity.Identifier(block) || version < 1 || revision < 1 || revision > 256 || !hashValid(digest) || !identity.Identifier(output) {
		return ErrInvalid
	}
	return nil
}

func requireMappingAuthoring(e identity.Envelope) error {
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	return access.Require(e, "charts.bind", access.Tenant(e, "read"))
}

// authoringBaseline loads the exact server-owned bytes only after independent
// ordinary read eligibility. Native Write/Preview reads preserve private actor,
// topic and dependency checks and give the domain internal SQL custody without
// exposing SQL through this lane or requiring a client SQL-read projection.
func (s *Authoring) authoringBaseline(ctx context.Context, e identity.Envelope, block string, version, revision int64, wantDigest string, mode Access) (*Service, Snapshot, error) {
	blocks, err := s.blockService()
	if err != nil {
		return nil, Snapshot{}, err
	}
	v, err := blocks.Read(ctx, e, block, Reference{Revision: revision})
	if err != nil {
		return nil, Snapshot{}, err
	}
	if v.State.Version != version || v.Revision != revision || v.Digest != wantDigest || v.State.Archived {
		return nil, Snapshot{}, store.ErrConflict
	}
	var base Snapshot
	if mode == Preview {
		repo, ok := blocks.repo.(AuthoringBlockCopyRepository)
		if !ok {
			return nil, Snapshot{}, ErrUnavailable
		}
		base, err = repo.ReadBlockForAuthoringCopy(ctx, e, block, Reference{Revision: revision})
	} else {
		base, err = blocks.repo.ReadBlock(ctx, e, block, Reference{Revision: revision}, mode)
	}
	if err != nil {
		return nil, Snapshot{}, err
	}
	if base.State.Version != version || base.Revision.Number != revision || base.Revision.Digest != wantDigest || DefinitionDigest(base.Revision.Definition) != wantDigest || base.State.Archived {
		return nil, Snapshot{}, store.ErrConflict
	}
	return blocks, base, nil
}

func amendAuthoringMapping(ctx context.Context, base Definition, output string, in AuthoringChartMapping, blocks *Service) (Definition, error) {
	d := clone(base)
	index := -1
	for i := range d.Outputs {
		if d.Outputs[i].ID == output {
			index = i
			break
		}
	}
	if index < 0 {
		return Definition{}, access.ErrNotFound
	}
	o := &d.Outputs[index]
	if o.Mapping == nil || o.Narrative != nil || o.Kind == "narrative" {
		return Definition{}, ErrInvalid
	}
	data := charts.Data{Version: charts.Version, Columns: authoringColumns(d.ExpectedSchema, *o.Mapping), Rows: [][]charts.Cell{}, Completeness: charts.Completeness{Status: "complete_result"}}
	var mapping charts.Mapping
	var err error
	if in.KPI != nil || in.Table != nil {
		mapping, err = charts.BindDisplay(ctx, data, in.Kind, in.Bindings, in.Order, in.Options, in.KPI, in.Table, chartLimits(blocks.limits))
	} else {
		mapping, err = charts.Bind(ctx, data, in.Kind, in.Bindings, in.Order, in.Options, chartLimits(blocks.limits))
	}
	if err != nil {
		if ctx.Err() != nil {
			return Definition{}, ctx.Err()
		}
		return Definition{}, ErrInvalid
	}
	o.Kind = "chart"
	if mapping.Kind == charts.Table {
		o.Kind = "table"
	}
	if mapping.Kind == charts.KPI {
		o.Kind = "kpi"
	}
	if o.Mapping.Presentation != nil {
		mapping.Presentation = clone(o.Mapping.Presentation)
		// Column reordering preserves the exact field overrides while storing
		// them in the newly selected canonical column order.
		previous := mapping.Presentation.Columns
		mapping.Presentation.Columns = make([]charts.ColumnDisplayOverride, 0, len(previous))
		for _, column := range mapping.Columns {
			for _, override := range previous {
				if override.Column == column.ID {
					mapping.Presentation.Columns = append(mapping.Presentation.Columns, override)
				}
			}
		}
		if len(mapping.Presentation.Columns) != len(previous) {
			return Definition{}, ErrInvalid
		}
		// A binding change must reject unsupported/dangling display overrides,
		// never silently erase or retarget them to a newly selected field.
		if _, err := charts.ProjectPresentationColumns(ctx, mapping, chartLimits(blocks.limits)); err != nil {
			return Definition{}, ErrInvalid
		}
	}
	o.Mapping = &mapping
	if in.Intent != nil {
		if d.SchemaVersion != CurrentSchemaVersion {
			return Definition{}, ErrInvalid
		}
		o.Intent = clone(in.Intent)
	}
	// Retain every declaration and derive only the selected output's obligation
	// bindings from its actual visible fields. A client cannot erase disclosure.
	o.AmountCompleteness = nil
	fields := amountOutputFields(*o)
	for _, a := range d.AmountCompleteness {
		if fields[a.ValueField.Name] {
			o.AmountCompleteness = append(o.AmountCompleteness, AmountOutputBinding{Declaration: a.ID, Role: "amount"})
		}
		if fields[a.UnknownCountField.Name] {
			o.AmountCompleteness = append(o.AmountCompleteness, AmountOutputBinding{Declaration: a.ID, Role: "unknown_count"})
		}
	}
	if err := validateDefinition(ctx, d, blocks.limits, true); err != nil {
		return Definition{}, err
	}
	return d, nil
}

// PatchBlockMapping targets only the current private draft. Native Edit appends
// an immutable revision and rechecks head CAS; schema-only binding neither runs
// a source nor transfers old validation evidence to the amendment.
func (s *Authoring) PatchBlockMapping(ctx context.Context, e identity.Envelope, in AuthoringBlockMappingRequest) (AuthoringBlockView, error) {
	return s.patchBlockDefinition(ctx, e, in, func(ctx context.Context, d Definition, blocks *Service) (Definition, error) {
		return amendAuthoringMapping(ctx, d, in.Output, in.Mapping, blocks)
	})
}

// patchBlockDefinition is the common admission and immutable commit boundary for
// selected-output edits. A presentation-only edit cannot choose another policy.
func (s *Authoring) patchBlockDefinition(ctx context.Context, e identity.Envelope, in AuthoringBlockMappingRequest, amend func(context.Context, Definition, *Service) (Definition, error)) (AuthoringBlockView, error) {
	if s == nil || ctx == nil {
		return AuthoringBlockView{}, ErrInvalid
	}
	if err := authoringMappingInput(in.Block, in.ExpectedVersion, in.Revision, in.Digest, in.Output); err != nil {
		return AuthoringBlockView{}, err
	}
	if err := requireMappingAuthoring(e); err != nil {
		return AuthoringBlockView{}, err
	}
	for _, a := range []Access{Read, Write} {
		if err := Require(e, in.Block, a); err != nil {
			return AuthoringBlockView{}, err
		}
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	blocks, base, err := s.authoringBaseline(ctx, e, in.Block, in.ExpectedVersion, in.Revision, in.Digest, Write)
	if err != nil {
		return AuthoringBlockView{}, err
	}
	if base.PublishedAt != nil || base.State.DraftRevision != in.Revision || base.State.DraftState == "rejected" {
		return AuthoringBlockView{}, store.ErrConflict
	}
	d, err := amend(ctx, base.Revision.Definition, blocks)
	if err != nil {
		return AuthoringBlockView{}, err
	}
	v, err := blocks.Edit(ctx, e, in.Block, EditRequest{ExpectedVersion: in.ExpectedVersion, Definition: d})
	if err != nil {
		return AuthoringBlockView{}, err
	}
	return authoringBlockView(v, blocks.limits), nil
}

// CopyBlockMapping uses native Preview access for internal full-definition
// custody, independently from ordinary read. Thus even a published source needs
// explicit preview reach for this operation. No source-write grant is inferred.
// Source version/archive eligibility is an admission-time snapshot check;
// target creation, not the source head, has commit-time create-CAS.
func (s *Authoring) CopyBlockMapping(ctx context.Context, e identity.Envelope, in AuthoringBlockCopyRequest) (AuthoringBlockView, error) {
	return s.copyBlockDefinition(ctx, e, in, func(ctx context.Context, d Definition, blocks *Service) (Definition, error) {
		return amendAuthoringMapping(ctx, d, in.Output, in.Mapping, blocks)
	})
}

func (s *Authoring) copyBlockDefinition(ctx context.Context, e identity.Envelope, in AuthoringBlockCopyRequest, amend func(context.Context, Definition, *Service) (Definition, error)) (AuthoringBlockView, error) {
	if s == nil || ctx == nil || !identity.Identifier(in.NewBlock) || in.NewBlock == in.Block {
		return AuthoringBlockView{}, ErrInvalid
	}
	if err := authoringMappingInput(in.Block, in.ExpectedVersion, in.Revision, in.Digest, in.Output); err != nil {
		return AuthoringBlockView{}, err
	}
	if err := requireMappingAuthoring(e); err != nil {
		return AuthoringBlockView{}, err
	}
	for _, a := range []Access{Read, Preview} {
		if err := Require(e, in.Block, a); err != nil {
			return AuthoringBlockView{}, err
		}
	}
	if err := Require(e, in.NewBlock, Write); err != nil {
		return AuthoringBlockView{}, err
	}
	if err := access.Require(e, Write.Action(), access.Tenant(e, "write")); err != nil {
		return AuthoringBlockView{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	blocks, base, err := s.authoringBaseline(ctx, e, in.Block, in.ExpectedVersion, in.Revision, in.Digest, Preview)
	if err != nil {
		return AuthoringBlockView{}, err
	}
	if err := RequireParent(e, base.State.Topic, Write, true); err != nil {
		return AuthoringBlockView{}, err
	}
	if err := RequireReferences(e, Write, base.References); err != nil {
		return AuthoringBlockView{}, err
	}
	d, err := amend(ctx, base.Revision.Definition, blocks)
	if err != nil {
		return AuthoringBlockView{}, err
	}
	_, refs, err := blocks.resolveDefinitions(ctx, e, d, false)
	if err != nil {
		return AuthoringBlockView{}, err
	}
	if _, err := blocks.resolveRules(ctx, e, d, false); err != nil {
		return AuthoringBlockView{}, err
	}
	// Provenance is copied exclusively from the authorized stored revision. The
	// fresh revision owns no validation/publication/attestation or retained data.
	provenance := clone(base.Revision.Provenance)
	provenance.Kind, provenance.ParentRevision = "copy", base.Revision.Number
	r, err := blocks.newRevision(e, 1, d, provenance)
	if err != nil {
		return AuthoringBlockView{}, err
	}
	state, err := blocks.commit(ctx, e, Mutation{ID: in.NewBlock, Topic: d.Topics[0].Topic, Kind: "create", Revision: &r, References: refs})
	if err != nil {
		return AuthoringBlockView{}, err
	}
	return authoringBlockView(project(Snapshot{State: state, Revision: r}, time.Now()), blocks.limits), nil
}

// AuthoringBlockValidateRequest explicitly permits bounded actual source work
// against one saved private draft. It is not an idempotency/retry contract.
type AuthoringBlockValidateRequest struct {
	Block           string     `json:"block"`
	ExpectedVersion int64      `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64      `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string     `json:"digest"`
	Arguments       []Argument `json:"arguments"`
	Resolution      Resolution `json:"resolution"`
}

type AuthoringBlockValidationResult struct {
	State    State                    `json:"state"`
	Evidence AuthoringBlockValidation `json:"evidence"`
}

func authoringBlockEvidence(e Evidence) AuthoringBlockValidation {
	return AuthoringBlockValidation{ID: e.ID, Revision: e.Revision, DefinitionDigest: e.DefinitionDigest, ExecutionDigest: e.ExecutionDigest, SchemaDigest: e.SchemaDigest, Schema: clone(e.Schema), CreatedAt: e.CreatedAt, ExpiresAt: e.ExpiresAt}
}

// ValidateBlock calls the native validator/executor explicitly. It never
// publishes and never interprets structural binding as observed-data evidence.
// An uncertain result must not be retried automatically: native validation has
// no caller operation key and a metadata read cannot attribute a later receipt
// to this exact request when another validation may have run concurrently.
func (s *Authoring) ValidateBlock(ctx context.Context, e identity.Envelope, in AuthoringBlockValidateRequest) (AuthoringBlockValidationResult, error) {
	if s == nil || ctx == nil || !identity.Identifier(in.Block) || in.ExpectedVersion < 1 || in.Revision < 1 || in.Revision > 256 || !hashValid(in.Digest) {
		return AuthoringBlockValidationResult{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return AuthoringBlockValidationResult{}, err
	}
	for _, a := range []Access{Read, Validate} {
		if err := Require(e, in.Block, a); err != nil {
			return AuthoringBlockValidationResult{}, err
		}
	}
	blocks, err := s.blockService()
	if err != nil {
		return AuthoringBlockValidationResult{}, err
	}
	view, err := blocks.Read(ctx, e, in.Block, Reference{Revision: in.Revision})
	if err != nil {
		return AuthoringBlockValidationResult{}, err
	}
	if view.State.Version != in.ExpectedVersion || view.Revision != in.Revision || view.Digest != in.Digest || !view.Private || view.State.Archived || view.State.DraftRevision != in.Revision || view.State.DraftState == "rejected" {
		return AuthoringBlockValidationResult{}, store.ErrConflict
	}
	result, err := blocks.Validate(ctx, e, in.Block, ValidateRequest{ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Arguments: in.Arguments, Resolution: in.Resolution})
	if err != nil {
		return AuthoringBlockValidationResult{}, err
	}
	return AuthoringBlockValidationResult{State: result.State, Evidence: authoringBlockEvidence(result.Evidence)}, nil
}
