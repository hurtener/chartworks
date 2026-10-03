package bifrost

import (
	"context"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	"sync"
	"testing"
	"time"
)

func TestRequestIsolationRoutingHeaders(t *testing.T) {
	cases := []*schemas.BifrostRequest{
		{RequestType: schemas.ChatCompletionRequest, ChatRequest: &schemas.BifrostChatRequest{Provider: schemas.OpenAI, Model: "original", Fallbacks: []schemas.Fallback{{Provider: schemas.OpenAI, Model: "fallback"}}}},
		{RequestType: schemas.EmbeddingRequest, EmbeddingRequest: &schemas.BifrostEmbeddingRequest{Provider: schemas.OpenAI, Model: "original"}},
		{RequestType: schemas.RerankRequest, RerankRequest: &schemas.BifrostRerankRequest{Provider: schemas.Cohere, Model: "original"}},
	}
	payload := "synthetic immutable payload"
	tokens := 16
	cases[0].ChatRequest.Input = []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &payload}}}
	cases[0].ChatRequest.Params = &schemas.ChatParameters{MaxCompletionTokens: &tokens}
	for _, in := range cases {
		t.Run(string(in.RequestType), func(t *testing.T) {
			out, short, err := (requestIsolation{}).PreLLMHook(nil, in)
			if err != nil || short != nil || out == in {
				t.Fatal("missing request isolation", err)
			}
			before, _ := json.Marshal(in)
			after, _ := json.Marshal(out)
			if string(before) != string(after) {
				t.Fatal("request payload or parameters changed")
			}
			p, m, _ := out.GetRequestFields()
			wantP, wantM, _ := in.GetRequestFields()
			if p != wantP || m != wantM {
				t.Fatal("routing changed")
			}
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				for i := 0; i < 100000; i++ {
					out.SetModel("worker")
				}
			}()
			go func() {
				defer wg.Done()
				for i := 0; i < 100000; i++ {
					_, model, _ := in.GetRequestFields()
					if model != "original" {
						t.Error("worker mutated timeout reader")
						return
					}
				}
			}()
			wg.Wait()
			if out.ChatRequest != nil {
				out.ChatRequest.Fallbacks[0].Model = "changed"
				if in.ChatRequest.Fallbacks[0].Model != "fallback" {
					t.Fatal("fallback headers shared")
				}
			}
		})
	}
	for _, bad := range []*schemas.BifrostRequest{nil, {}, {RequestType: schemas.ChatCompletionRequest}, {RequestType: schemas.EmbeddingRequest}, {RequestType: schemas.RerankRequest}} {
		_, short, err := (requestIsolation{}).PreLLMHook(nil, bad)
		if err != nil || short == nil || short.Error == nil {
			t.Fatal("unsupported request reached provider")
		}
	}
}

func TestRequestIsolationCannotBeSkippedByParentContext(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	parent, cancel := context.WithDeadline(context.WithValue(context.Background(), schemas.BifrostContextKeySkipPluginPipeline, true), deadline)
	parent = context.WithValue(parent, schemas.BifrostContextKeyPassthroughExtraParams, true)
	parent = context.WithValue(parent, schemas.BifrostContextKeyUseRawRequestBody, true)
	parent = context.WithValue(parent, schemas.BifrostContextKeyLargePayloadMode, true)
	ctx, closeCtx := isolatedBifrostContext(parent)
	defer closeCtx()
	if got, ok := ctx.Deadline(); !ok || !got.Equal(deadline) {
		t.Fatal("deadline changed", got, ok)
	}
	if skipped, ok := ctx.Value(schemas.BifrostContextKeySkipPluginPipeline).(bool); !ok || skipped {
		t.Fatal("inherited SDK flag bypassed isolation")
	}
	if passthrough, _ := ctx.Value(schemas.BifrostContextKeyPassthroughExtraParams).(bool); passthrough {
		t.Fatal("inherited passthrough flag")
	}
	for _, key := range []schemas.BifrostContextKey{schemas.BifrostContextKeyUseRawRequestBody, schemas.BifrostContextKeyLargePayloadMode} {
		if flag, _ := ctx.Value(key).(bool); flag {
			t.Fatal("inherited body substitution flag")
		}
	}
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("cancellation detached")
	}
}
