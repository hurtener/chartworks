package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/topicfeedback"
	"github.com/hurtener/chartworks/internal/topicfeedbackapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func topicProposalResponse(t *testing.T) string {
	t.Helper()
	content := `{"edits":[{"kind":"measure","id":"revenue","aggregation":"average","description":"Average order amount, not the total."}]}`
	raw, err := json.Marshal(map[string]any{"id": "synthetic-semantic-proposal", "object": "chat.completion", "model": "model-enhance", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 20, "total_tokens": 40}})
	if err != nil {
		t.Fatal(err)
	}
	return "chat_raw:" + string(raw)
}

// This recorded-provider fixture crosses real metadata/native source boundaries.
// It does not claim a live provider quality measurement.
func TestTopicFeedbackProposalLifecycle(t *testing.T)      { topicFeedbackLifecycle(t, false) }
func TestTopicFeedbackVocabularyLifecycle(t *testing.T)    { topicFeedbackLifecycle(t, true) }
func TestTopicFeedbackVocabularyProfileFence(t *testing.T) { topicFeedbackLifecycle(t, true, true) }
func topicFeedbackLifecycle(t *testing.T, vocabulary bool, staleProfile ...bool) {
	fixture := newPhase18FixtureReviewed(t, true)
	query, published := newPhase18Service(t, fixture)
	ctx := context.Background()
	e := phase18Envelope(t, fixture, fixture.f.e.User(), fixture.f.e.Session(), true)
	draftService, err := drafts.NewWithEngine(fixture.f.db, fixture.f.s, fixture.f.service, fixture.model.engine)
	if err != nil {
		t.Fatal(err)
	}
	service, err := topicfeedback.New(fixture.f.db, query, draftService, fixture.model.engine)
	if err != nil {
		t.Fatal(err)
	}
	if vocabulary {
		// Establish a real current profile containing the new filter field before
		// query feedback exists. This is physical authoring, not model-supplied scope.
		datasetIndex := 0
		for i, d := range fixture.pack.Datasets {
			for _, m := range fixture.pack.Measures {
				if m.ID == "revenue" && m.Field.Dataset == d.ID {
					datasetIndex = i
				}
			}
		}
		old := fixture.pack.Datasets[datasetIndex]
		profile := fixture.f.profile(t, engineering.ProfileSpec{ID: "feedback-vocabulary-profile", Previous: old.Source.ProfileVersion, Source: old.Source.Source, Context: old.Source.Context, Dataset: old.ID, Columns: []string{"id", "amount", "name"}, SkipLLM: true}).Profile.Profile
		dataset := old
		dataset.Source.ProfileVersion = profile.Version
		dataset.Source.ProfileDigest = profile.DeterministicHash()
		dataset.Columns = nil
		for _, c := range profile.Schema {
			if c.Name == "id" || c.Name == "amount" || c.Name == "name" {
				dataset.Columns = append(dataset.Columns, semantics.Column{ID: c.Name, SourceName: c.Name, Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable, Sensitivity: semantics.LiteralNonSensitive})
			}
		}
		fixture.pack.Datasets[datasetIndex] = dataset
		fixture.pack.Version = "vocabulary-base"
		prior, err := draftService.Read(ctx, e, fixture.pack.Topic, 0)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := draftService.Save(ctx, e, drafts.SaveRequest{Expected: prior.Metadata.Revision, Pack: fixture.pack, Change: "Explicitly classify synthetic profile fields for vocabulary authoring"})
		if err != nil {
			t.Fatal("prepare vocabulary draft", err)
		}
		review, err := published.Review(ctx, e, fixture.pack.Topic, topics.ReviewRequest{DraftRevision: saved.Metadata.Revision, Digest: saved.Metadata.Digest, Decision: "approve", Note: "Synthetic source field classification"})
		if err != nil {
			t.Fatal(err)
		}
		priorPublication, err := published.Read(ctx, e, fixture.pack.Topic, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = published.Publish(ctx, e, fixture.pack.Topic, topics.PublishRequest{Expected: priorPublication.State.Revision, Review: review.ID}); err != nil {
			t.Fatal("publish vocabulary base", err)
		}
	}
	fixture.model.embeddingMode.Store("fixed")
	fixture.model.rerankMode.Store("fixed")
	originalSQL := "SELECT sum(amount) AS value FROM analytics.sales"
	correctedSQL := "SELECT avg(amount) AS value FROM analytics.sales"
	if vocabulary {
		correctedSQL = "SELECT avg(amount) FILTER (WHERE name = 'one') AS value FROM analytics.sales"
	}
	fixture.model.mode.Store(phase18RawResponse(t, originalSQL))
	question := phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)
	question.MetricIDs = []string{"revenue"}
	planned, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: question, Operation: "semantic-proposal-base"})
	if err != nil {
		t.Fatal("base plan", err)
	}
	const privateNote = "PRIVATE_OLD_FEEDBACK_VALUE_93851"
	if err = query.Feedback(ctx, e, nlqexec.FeedbackRequest{QueryID: planned.QueryID, Verdict: "negative", Correction: "SELECT avg(amount) AS value FROM analytics.sales", Note: privateNote}); err != nil {
		t.Fatal("feedback", err)
	}
	registry, err := topicfeedbackapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, topicfeedbackapi.Handler(fixture.model.token.verifier, service, http.NotFoundHandler()))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		return phase18Token(t, fixture, e.User(), e.Session(), true), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	bearerHTTP := phase18Token(t, fixture, e.User(), e.Session(), true)
	for _, bad := range []struct {
		media, path, body string
		status            int
	}{
		{"text/plain", "/v1/topic-feedback/read", `{"id":"missing"}`, 400},
		{"application/json", "/v1/topic-feedback/read", `{"id":"missing","grant":"all"}`, 400},
		{"application/json", "/v1/topic-feedback/read?extra=1", `{"id":"missing"}`, 400},
		{"application/json", "/v1/topic-feedback/read", strings.Repeat("x", (16<<10)+1), 413},
	} {
		request := httptest.NewRequest("POST", bad.path, strings.NewReader(bad.body))
		request.Header.Set("Authorization", "Bearer "+bearerHTTP)
		request.Header.Set("Content-Type", bad.media)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != bad.status {
			t.Fatal("HTTP closed input", recorder.Code, recorder.Body.String())
		}
	}
	refs, err := client.ListTopicFeedbackEvidence(ctx, planned.QueryID)
	if err != nil || len(refs) != 1 || !refs[0].Corrected {
		t.Fatal("discover exact feedback", refs, err)
	}
	current, err := draftService.Read(ctx, e, fixture.pack.Topic, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, err := published.Read(ctx, e, fixture.pack.Topic, "")
	if err != nil {
		t.Fatal(err)
	}
	fixture.model.mu.Lock()
	first := len(fixture.model.requestBodies)
	fixture.model.mu.Unlock()
	proof, proofErr := query.SemanticFeedbackEvidence(ctx, e, refs[0].ID)
	if proofErr != nil {
		t.Fatal("evidence admission", proofErr)
	}
	if _, _, proofErr = proof.Inspect(e); proofErr != nil {
		t.Fatal("evidence seal", proofErr)
	}
	if _, _, proofErr = draftService.PrepareFeedbackAuthoring(ctx, e, fixture.pack.Topic, current.Metadata.Revision); proofErr != nil {
		t.Fatal("authoring admission", proofErr)
	}
	fixture.model.mode.Store(topicProposalResponse(t))
	req := sdk.TopicFeedbackProposeRequest{ID: "reviewed-meaning-proposal", Topic: fixture.pack.Topic, FeedbackID: refs[0].ID, Expected: current.Metadata.Revision, AuthorIntent: "The selected revenue concept should represent average order amount instead of the total. Propose the aggregate correction for review."}
	if vocabulary {
		var source semantics.SourceReference
		var dataset string
		for _, d := range current.Pack.Datasets {
			for _, c := range d.Columns {
				if c.ID == "name" {
					source = d.Source
					dataset = d.ID
				}
			}
		}
		req.Vocabulary = []drafts.AuthoringValue{{ID: "admitted_one", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: "name"}, Origin: source, Kind: "text", Value: "one", Sensitivity: semantics.LiteralNonSensitive, Nulls: "exclude"}}
		req.AuthorIntent = "Average order amount for the explicitly supplied one population. Propose a reviewed population filter."
		response := strings.Replace(topicProposalResponse(t), `\"aggregation\":\"average\"`, `\"aggregation\":\"average\",\"new_filters\":[{\"measure\":\"revenue\",\"id\":\"admitted_population\",\"operator\":\"eq\",\"nulls\":\"exclude\",\"vocabulary_ids\":[\"admitted_one\"]}]`, 1)
		fixture.model.mode.Store(response)
		bad := req
		bad.ID = "foreign-vocabulary"
		bad.Vocabulary = append([]drafts.AuthoringValue(nil), req.Vocabulary...)
		bad.Vocabulary[0].Origin.ProfileVersion = "foreign-profile"
		if _, err := service.Propose(ctx, e, bad); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("foreign catalog", err)
		}
	}
	proposal, err := client.ProposeTopicFeedback(ctx, req)
	if err != nil {
		_, domainErr := service.Propose(ctx, e, req)
		t.Fatalf("propose: %#v domain=%v", err, domainErr)
	}
	if proposal.State != "proposed" || proposal.Digest == "" || len(proposal.Edits) != 1 {
		t.Fatal("proposal", proposal)
	}
	fixture.model.mu.Lock()
	bodies := append([]string(nil), fixture.model.requestBodies[first:]...)
	calls := len(fixture.model.requestBodies)
	fixture.model.mu.Unlock()
	for _, body := range bodies {
		if strings.Contains(body, privateNote) || strings.Contains(body, correctedSQL) || strings.Contains(body, "SELECT avg(amount) AS value FROM analytics.sales") || strings.Contains(body, originalSQL) {
			t.Fatal("historical private content reached proposal model")
		}
	}
	if vocabulary {
		foundVocabulary := false
		for _, body := range bodies {
			if strings.Contains(body, "admitted_one") {
				foundVocabulary = true
			}
		}
		if !foundVocabulary {
			t.Fatal("admitted vocabulary did not reach model packet")
		}
		changed := req
		changed.Vocabulary = append([]drafts.AuthoringValue(nil), req.Vocabulary...)
		changed.Vocabulary[0].Value = "two"
		if _, err := service.Propose(ctx, e, changed); !errors.Is(err, store.ErrConflict) {
			t.Fatal("catalog idempotency mismatch", err)
		}
	}
	again, err := client.ProposeTopicFeedback(ctx, req)
	if err != nil || again.Digest != proposal.Digest {
		t.Fatal("idempotent proposal", again, err)
	}
	fixture.model.mu.Lock()
	afterCalls := len(fixture.model.requestBodies)
	fixture.model.mu.Unlock()
	if afterCalls != calls {
		t.Fatal("retry repeated inference")
	}
	changedReq := req
	changedReq.AuthorIntent = "Conflicting author intent"
	if _, err = service.Propose(ctx, e, changedReq); !errors.Is(err, store.ErrConflict) {
		t.Fatal("idempotency mismatch", err)
	}
	foreign := phase18Envelope(t, fixture, e.User(), "other-session", true)
	if _, err = service.Read(ctx, foreign, topicfeedback.ReadRequest{ID: proposal.ID}); !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("foreign read", err)
	}
	wrong := sdk.TopicFeedbackApplyRequest{ID: proposal.ID, Digest: strings.Repeat("0", 64)}
	if _, err = service.Apply(ctx, e, wrong); !errors.Is(err, store.ErrConflict) {
		t.Fatal("wrong proposal digest", err)
	}
	// A separate proposal on the same draft becomes stale after the first apply.
	staleReq := req
	staleReq.ID = "stale-meaning-proposal"
	stale, err := client.ProposeTopicFeedback(ctx, staleReq)
	if err != nil {
		t.Fatal(err)
	}
	still, err := published.Read(ctx, e, fixture.pack.Topic, "")
	if err != nil || still.Digest != before.Digest {
		t.Fatal("proposal mutated publication", err)
	}
	if len(staleProfile) > 0 && staleProfile[0] {
		origin := req.Vocabulary[0].Origin
		fixture.f.profile(t, engineering.ProfileSpec{ID: "feedback-new-profile-head", Previous: origin.ProfileVersion, Source: origin.Source, Context: origin.Context, Dataset: origin.Dataset, Columns: []string{"id", "amount", "name"}, SkipLLM: true})
		if _, err := service.Apply(ctx, e, topicfeedback.ApplyRequest{ID: proposal.ID, Digest: proposal.Digest}); err == nil {
			t.Fatal("stale profile catalog applied")
		}
		retained, err := draftService.Read(ctx, e, req.Topic, 0)
		if err != nil || retained.Metadata.Revision != current.Metadata.Revision {
			t.Fatal("failed apply changed private draft", err)
		}
		active, err := published.Read(ctx, e, req.Topic, "")
		if err != nil || active.Digest != before.Digest {
			t.Fatal("failed apply changed active semantics", err)
		}
		return
	}
	applied, err := client.ApplyTopicFeedback(ctx, sdk.TopicFeedbackApplyRequest{ID: proposal.ID, Digest: proposal.Digest})
	if err != nil {
		t.Fatal("apply", err)
	}
	if applied.Metadata.Revision != current.Metadata.Revision+1 || applied.Quality != nil || applied.Pack.Measures[0].Aggregation != semantics.AggregationAverage {
		t.Fatal("application not ordinary corrected draft", applied)
	}
	replay, err := client.ApplyTopicFeedback(ctx, sdk.TopicFeedbackApplyRequest{ID: proposal.ID, Digest: proposal.Digest})
	if err != nil || replay.Metadata.Digest != applied.Metadata.Digest {
		t.Fatal("apply replay", err)
	}

	if _, err = service.Apply(ctx, e, topicfeedback.ApplyRequest{ID: stale.ID, Digest: stale.Digest}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale draft applied", err)
	}
	still, err = published.Read(ctx, e, fixture.pack.Topic, "")
	if err != nil || still.Digest != before.Digest {
		t.Fatal("apply bypassed publication", err)
	}
	review, err := published.Review(ctx, e, fixture.pack.Topic, topics.ReviewRequest{DraftRevision: applied.Metadata.Revision, Digest: applied.Metadata.Digest, Decision: "approve", Note: "Explicitly reviewed aggregate correction"})
	if err != nil {
		t.Fatal(err)
	}
	active, err := published.Publish(ctx, e, fixture.pack.Topic, topics.PublishRequest{Expected: before.State.Revision, Review: review.ID})
	if err != nil {
		t.Fatal("publish reviewed proposal", err)
	}
	if vocabulary && (len(active.Definition.Measures[0].Filters) != 1 || active.Definition.Measures[0].Filters[0].Values[0] != "one") {
		t.Fatal("new population filter missing")
	}
	if active.Definition.Measures[0].Aggregation != semantics.AggregationAverage || active.Digest == before.Digest {
		t.Fatal("publication did not consume correction")
	}
	fixture.model.mode.Store(phase18RawResponse(t, correctedSQL))
	fixture.model.mu.Lock()
	first = len(fixture.model.requestBodies)
	fixture.model.mu.Unlock()
	later, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: question, Operation: "semantic-proposal-consumer"})
	if err != nil {
		t.Fatal("later generation", err)
	}
	run, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: later.QueryID, Operation: "semantic-proposal-result", Rows: 10, Bytes: 4096})
	if err != nil || run.Execution.Result == nil {
		t.Fatal("later execution", err)
	}
	var expected string
	if err = fixture.f.admin.QueryRow(ctx, func() string {
		if vocabulary {
			return "SELECT avg(amount)::text FROM analytics.sales WHERE name = 'one'"
		}
		return "SELECT avg(amount)::text FROM analytics.sales"
	}()).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	if !liveSingleNumericEquals(run.Execution.Result.Rows, expected) {
		t.Fatal("corrected semantic result", run.Execution.Result.Rows, expected)
	}
	fixture.model.mu.Lock()
	bodies = append([]string(nil), fixture.model.requestBodies[first:]...)
	fixture.model.mu.Unlock()
	found := false
	for _, body := range bodies {
		if strings.Contains(body, "average") && strings.Contains(body, "revenue") {
			found = true
		}
	}
	if !found {
		t.Fatal("published correction absent from later model prompt")
	}
	sc, _ := store.NewScope(e.Tenant(), e.User())
	examples, err := fixture.f.db.ListExamples(ctx, sc, fixture.pack.Topic, 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range examples {
		if example.Origin.TopicVersion != before.State.Version || example.State == "active" {
			t.Fatal("publication silently requalified historical example")
		}
	}
	if _, err = service.Apply(ctx, e, topicfeedback.ApplyRequest{ID: stale.ID, Digest: stale.Digest}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("old publication proposal not fenced", err)
	}
	readBack, readErr := client.ReadTopicFeedback(ctx, proposal.ID)
	if readErr != nil || readBack.State != "applied" {
		t.Fatal("SDK retained proposal", readErr)
	}
	bindings, err := topicfeedbackapi.MCPBindings(service)
	if err != nil || len(bindings) != 4 {
		t.Fatal("MCP consumer registration", len(bindings), err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcpserver.New(fixture.model.token.verifier, mcpRegistry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	claims := fixture.model.token.claims(e.Tenant(), e.User(), append(e.Scopes(), "mcp.use"))
	claims["session"] = e.Session()
	claims["aud"] = fixture.model.token.cfg.MCPAudience()
	bearer := fixture.model.token.sign(t, claims, nil)
	mcpClient, err := mcpServer.Client(func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(topicfeedback.ReadRequest{ID: proposal.ID})
	result, err := mcpClient.CallTool(ctx, "read_topic_feedback", raw)
	if err != nil || result == nil || result.IsError {
		t.Fatal("MCP retained proposal consumer", result, err)
	}
	raw, _ = json.Marshal(topicfeedback.ApplyRequest{ID: proposal.ID, Digest: proposal.Digest})
	result, err = mcpClient.CallTool(ctx, "apply_topic_feedback", raw)
	if err != nil || result == nil || result.IsError {
		t.Fatal("MCP apply idempotent consumer", result, err)
	}

}

// Report deletion erases owned data while separately authored semantics have
// an explicit content-free tombstone and cannot silently reacquire an origin.
func TestTopicFeedbackDocumentRetentionBoundary(t *testing.T) {
	for _, mode := range []string{"pending", "applied", "concurrent_apply", "concurrent_create"} {
		t.Run(mode, func(t *testing.T) {
			f := newPhase29Execution(t, true)
			ctx := t.Context()
			// Use the existing private topic author's session for both the report and
			// its dynamic query; do not rewrite private draft/profile ownership.
			session := f.f.f.e.Session()
			var err error
			f.execute, err = f.f.f.token.verifier.Verify(ctx, phase27Token(t, f.f, f.execute.User(), session, f.execute.Scopes()), auth.HTTP)
			if err != nil {
				t.Fatal(err)
			}
			f.author, err = f.f.f.token.verifier.Verify(ctx, phase27Token(t, f.f, f.author.User(), session, f.author.Scopes()), auth.HTTP)
			if err != nil {
				t.Fatal(err)
			}
			definition := phase29Text("Feedback retention report")
			definition.Widgets = append(definition.Widgets, f.queryWidget())
			document := f.report(t, "feedback-retention-report", definition, true)
			run, err := f.compositions.Admit(ctx, f.execute, "report", document.ID, reporting.CompositionRequest{Key: "feedback-retention-run"})
			if err != nil {
				t.Fatal(err)
			}
			completed, err := f.compositions.Run(ctx, f.execute, run.ID, false)
			if err != nil || !completed.Complete {
				t.Fatal("real dynamic report query", completed, err)
			}
			raw := support.Raw(t, f.f.f.dsn)
			var queryID string
			if err = raw.QueryRow(ctx, `SELECT query_id FROM chartworks.nlq_queries WHERE tenant_id=$1 AND operation LIKE $2 ORDER BY query_id LIMIT 1`, f.execute.Tenant(), "composition:"+run.ID+":%").Scan(&queryID); err != nil {
				t.Fatal(err)
			}
			author := phase18Envelope(t, f.f, f.execute.User(), session, true)
			const erasedNote = "PRIVATE_REPORT_NOTE_TO_ERASE"
			if err = f.query.Feedback(ctx, author, nlqexec.FeedbackRequest{QueryID: queryID, Verdict: "negative", Note: erasedNote}); err != nil {
				t.Fatal("document feedback", err)
			}
			var feedbackID string
			if err = raw.QueryRow(ctx, `SELECT feedback_id FROM chartworks.nlq_feedback WHERE tenant_id=$1 AND query_id=$2`, author.Tenant(), queryID).Scan(&feedbackID); err != nil {
				t.Fatal(err)
			}
			draftService, err := drafts.NewWithEngine(f.f.f.db, f.f.f.s, f.f.f.service, f.f.model.engine)
			if err != nil {
				t.Fatal(err)
			}
			current, err := draftService.Read(ctx, author, f.f.pack.Topic, 0)
			if err != nil {
				t.Fatal("owned draft", err)
			}
			service, err := topicfeedback.New(f.f.f.db, f.query, draftService, f.f.model.engine)
			if err != nil {
				t.Fatal(err)
			}
			f.f.model.mode.Store(topicProposalResponse(t))
			request := topicfeedback.ProposeRequest{ID: "document-proposal", Topic: f.f.pack.Topic, FeedbackID: feedbackID, Expected: current.Metadata.Revision, AuthorIntent: "Separately author a reviewed average order amount definition"}
			var proposal topicfeedback.Proposal
			if mode != "concurrent_create" {
				proposal, err = service.Propose(ctx, author, request)
				if err != nil {
					t.Fatal("document-origin proposal", err)
				}
			}
			apply := func() error {
				_, err := service.Apply(ctx, author, topicfeedback.ApplyRequest{ID: proposal.ID, Digest: proposal.Digest})
				return err
			}
			erase := func() error {
				deleted, err := f.documents.Delete(ctx, f.author, "report", document.ID, reporting.DocumentDeleteRequest{ExpectedVersion: document.Version, Key: "feedback-retention-delete", Reason: "Erase report-owned query evidence"})
				if err == nil && deleted.ErasedQueries != 1 {
					return errors.New("owned query not erased")
				}
				return err
			}
			applied := false
			switch mode {
			case "applied":
				if err = apply(); err != nil {
					t.Fatal(err)
				}
				applied = true
				if err = erase(); err != nil {
					t.Fatal(err)
				}
			case "concurrent_apply":
				start := make(chan struct{})
				a, d := make(chan error, 1), make(chan error, 1)
				go func() { <-start; a <- apply() }()
				go func() { <-start; d <- erase() }()
				close(start)
				ae, de := <-a, <-d
				if de != nil {
					t.Fatal("concurrent deletion", de)
				}
				applied = ae == nil
				if ae != nil && !errors.Is(ae, nlqexec.ErrFeedbackRetention) && !errors.Is(ae, store.ErrNotFound) && !errors.Is(ae, store.ErrConflict) && !errors.Is(ae, access.ErrNotFound) {
					t.Fatal("concurrent application", ae)
				}
			case "concurrent_create":
				type outcome struct {
					p topicfeedback.Proposal
					e error
				}
				start := make(chan struct{})
				a, d := make(chan outcome, 1), make(chan error, 1)
				go func() { <-start; p, e := service.Propose(ctx, author, request); a <- outcome{p, e} }()
				go func() { <-start; d <- erase() }()
				close(start)
				made, de := <-a, <-d
				if de != nil {
					t.Fatal("concurrent create deletion", de)
				}
				proposal = made.p
				if made.e != nil && !errors.Is(made.e, store.ErrNotFound) && !errors.Is(made.e, access.ErrNotFound) && !errors.Is(made.e, store.ErrConflict) {
					t.Fatal("concurrent proposal", made.e)
				}
			default:
				if err = erase(); err != nil {
					t.Fatal(err)
				}
			}
			var remaining int
			if err = raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_feedback WHERE tenant_id=$1 AND feedback_id=$2`, author.Tenant(), feedbackID).Scan(&remaining); err != nil || remaining != 0 {
				t.Fatal("owned feedback retained", remaining, err)
			}
			if err = raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_queries WHERE tenant_id=$1 AND query_id=$2`, author.Tenant(), queryID).Scan(&remaining); err != nil || remaining != 0 {
				t.Fatal("owned query retained", remaining, err)
			}
			if proposal.ID != "" {
				retained, err := service.Read(ctx, author, topicfeedback.ReadRequest{ID: proposal.ID})
				if err != nil || !retained.OriginErased {
					t.Fatal("missing detached origin tombstone", retained, err)
				}
				bytes, _ := json.Marshal(retained)
				if strings.Contains(string(bytes), erasedNote) {
					t.Fatal("erased report content retained")
				}
				if applied {
					if err = apply(); err != nil {
						t.Fatal("independently applied draft lost", err)
					}
				} else if err = apply(); !errors.Is(err, nlqexec.ErrFeedbackRetention) {
					t.Fatal("erased origin applied", err)
				}
			}
		})
	}
}
