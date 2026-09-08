package api

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

type requestDTO struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}
type responseDTO struct {
	Values   []requestDTO `json:"values"`
	Observed time.Time    `json:"observed"`
	Optional string       `json:"optional,omitempty"`
}

func definition(t *testing.T) Definition {
	t.Helper()
	req, err := SchemaFor("request", reflect.TypeFor[requestDTO](), false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := SchemaFor("response", reflect.TypeFor[responseDTO](), true)
	if err != nil {
		t.Fatal(err)
	}
	return Definition{Operation: Operation{"POST", "/v1/sources/{id}/test", "sources.read", "warehouse_catalog_read"}, ID: "testSource", Summary: "Test source", ResourceLoader: "sources.Service.Test", Audit: "read_only_no_domain_audit", MaxBodyBytes: 65536, Request: req, Response: res, Errors: []ErrorResponse{{401, "unauthenticated"}, {503, "unavailable"}, {409, "conflict"}, {409, "context_changed"}}}
}

func TestSchemaTracksClosedRequestAndNullableResponseShapes(t *testing.T) {
	d := definition(t)
	for _, value := range []string{`{"name":"x","values":[]}`, `{"name":"x","values":["one"]}`} {
		if err := d.Request.Validate([]byte(value), 65536); err != nil {
			t.Fatalf("valid request rejected: %v", err)
		}
	}
	for _, value := range []string{`null`, `{}`, `{"name":"x","values":null}`, `{"Name":"x","values":[]}`, `{"name":"x","values":[],"tenant":"other"}`, `{"name":"x","name":"y","values":[]}`} {
		if d.Request.Validate([]byte(value), 65536) == nil {
			t.Fatalf("request shape accepted %s", value)
		}
	}
	for _, value := range []string{`{"values":null,"observed":"2026-09-07T12:00:00Z"}`, `{"values":[{"name":"x","values":null}],"observed":"2026-09-07T12:00:00Z","optional":"ok"}`} {
		if err := d.Response.Validate([]byte(value), 65536); err != nil {
			t.Fatalf("actual Go response shape rejected: %v", err)
		}
	}
	if d.Response.Validate([]byte(`{"values":[],"observed":"2026-09-07T12:00:00Z","secret":"x"}`), 65536) == nil {
		t.Fatal("response admitted undeclared field")
	}
	for _, typ := range []reflect.Type{nil, reflect.TypeFor[map[string]string](), reflect.TypeFor[[]byte](), reflect.TypeFor[*requestDTO](), reflect.TypeFor[struct{ Untagged string }](), reflect.TypeFor[struct {
		Hidden string `json:"-"`
	}]()} {
		if _, err := SchemaFor("bad", typ, false); !errors.Is(err, ErrRegistration) {
			t.Fatalf("unsupported shape %v: %v", typ, err)
		}
	}
}

func TestRegistryRequiresCompleteUniqueDefinitions(t *testing.T) {
	for _, edit := range []func(*Definition){
		func(d *Definition) { d.ID = "" }, func(d *Definition) { d.Summary = "" }, func(d *Definition) { d.ResourceLoader = "" }, func(d *Definition) { d.Audit = "" }, func(d *Definition) { d.Action = "" }, func(d *Definition) { d.Effect = "" },
		func(d *Definition) { d.Path = "/v1/sources/{id}/{id}" }, func(d *Definition) { d.Path = "/v1/sources/" }, func(d *Definition) { d.Path = "/v1/sources/{unknown}" }, func(d *Definition) { d.Path = "/other/sources" },
		func(d *Definition) { d.Method = "TRACE" }, func(d *Definition) { d.Method = "GET" }, func(d *Definition) { d.Request = nil }, func(d *Definition) { d.Request = &gateway.Schema{} }, func(d *Definition) { d.Response = nil }, func(d *Definition) { d.Response = &gateway.Schema{} }, func(d *Definition) { d.MaxBodyBytes = 0 },
		func(d *Definition) { d.Errors = nil }, func(d *Definition) { d.Errors[0].Status = 200 }, func(d *Definition) { d.Errors[0].Code = "" }, func(d *Definition) { d.Errors = append(d.Errors, d.Errors[0]) },
	} {
		d := definition(t)
		edit(&d)
		if _, err := New([]Definition{d}); !errors.Is(err, ErrRegistration) {
			t.Fatalf("incomplete registration passed: %#v %v", d, err)
		}
	}
	if _, err := New(nil); err == nil {
		t.Fatal("empty registry")
	}
	d := definition(t)
	if _, err := New([]Definition{d, d}); err == nil {
		t.Fatal("duplicate registration")
	}
	other := d
	other.ID = "other"
	if _, err := New([]Definition{d, other}); err == nil {
		t.Fatal("duplicate method/path")
	}
	other = d
	other.Path = "/v1/other"
	if _, err := New([]Definition{d, other}); err == nil {
		t.Fatal("duplicate operation ID")
	}
}

func TestRegistryDetachesSchemaWrappersAtEveryBoundary(t *testing.T) {
	for _, boundary := range []string{"constructor", "definitions", "match"} {
		t.Run(boundary, func(t *testing.T) {
			input := definition(t)
			r, err := New([]Definition{input})
			if err != nil {
				t.Fatal(err)
			}
			before, err := r.OpenAPI("Sources", "1")
			if err != nil {
				t.Fatal(err)
			}
			exposed := input
			switch boundary {
			case "definitions":
				exposed = r.Definitions()[0]
			case "match":
				exposed, _, _ = r.Match("POST", "/v1/sources/source1/test")
			}
			// Schema's fields are private, but callers can overwrite its wrapper.
			*exposed.Request = gateway.Schema{}
			*exposed.Response = gateway.Schema{}
			retained := r.Definitions()[0]
			if err := retained.Request.Validate([]byte(`{"name":"x","values":[]}`), 65536); err != nil {
				t.Errorf("request schema corrupted through %s: %v", boundary, err)
			}
			if err := retained.Response.Validate([]byte(`{"values":null,"observed":"2026-09-07T12:00:00Z"}`), 65536); err != nil {
				t.Errorf("response schema corrupted through %s: %v", boundary, err)
			}
			after, err := r.OpenAPI("Sources", "1")
			if err != nil || string(after) != string(before) {
				t.Errorf("OpenAPI changed through %s: %v", boundary, err)
			}
		})
	}
}

func TestRegistryRoutingAndOpenAPIUseSameDetachedDefinitions(t *testing.T) {
	d := definition(t)
	get := d
	get.Operation = Operation{"GET", "/v1/sources", "sources.read", "metadata_read"}
	get.ID = "listSources"
	get.Request = nil
	get.MaxBodyBytes = 0
	r, err := New([]Definition{get, d})
	if err != nil {
		t.Fatal(err)
	}
	d.Errors[0].Code = "mutated"
	got, id, known := r.Match("POST", "/v1/sources/source1/test")
	if !known || id != "source1" || got.ID != "testSource" || got.Errors[0].Code != "unauthenticated" {
		t.Fatalf("match=%#v %q %v", got, id, known)
	}
	got.Errors[0].Code = "changed"
	if op, _, known := r.Match("GET", "/v1/sources/source1/test"); !known || op.ID != "" {
		t.Fatal("lost known-path method mismatch")
	}
	for _, path := range []string{"/unregistered", "/v1/sources/bad%2Fid/test", "/v1/sources//test", "/v1/sources/source1/test/"} {
		if _, _, known := r.Match("POST", path); known {
			t.Fatalf("unexpected route %s", path)
		}
	}
	operations := r.Operations()
	operations[0].Path = "changed"
	definitions := r.Definitions()
	definitions[0].Errors[0].Code = "changed"
	if r.Operations()[0].Path != "/v1/sources" || r.Definitions()[0].Errors[0].Code != "unauthenticated" {
		t.Fatal("mutable registry state escaped")
	}
	wire, err := r.OpenAPI("Sources", "1")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if json.Unmarshal(wire, &doc) != nil {
		t.Fatal("invalid generated JSON")
	}
	paths := doc["paths"].(map[string]any)
	if len(paths) != 2 || doc["openapi"] != "3.1.1" {
		t.Fatal("document not derived from registry")
	}
	op := paths[d.Path].(map[string]any)["post"].(map[string]any)
	if op["operationId"] != d.ID || op["x-chartworks-action"] != d.Action || op["x-chartworks-max-body-bytes"] != float64(65536) {
		t.Fatalf("operation metadata missing: %#v", op)
	}
	if strings.Contains(string(wire), "$ref") || strings.Contains(string(wire), `"mutated"`) || strings.Contains(string(wire), `"changed"`) {
		t.Fatal("schema is not self-contained/detached")
	}
	if _, err = r.OpenAPI("", "1"); err == nil {
		t.Fatal("invalid document info")
	}
	var zero *Registry
	if zero.Operations() != nil || zero.Definitions() != nil {
		t.Fatal("zero registry")
	}
	if _, _, known := zero.Match("GET", "/"); known {
		t.Fatal("zero match")
	}
	if _, err := zero.OpenAPI("Sources", "1"); err == nil {
		t.Fatal("zero document")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				again, err := r.OpenAPI("Sources", "1")
				if err != nil || string(again) != string(wire) {
					t.Error("concurrent document changed")
				}
				defs := r.Definitions()
				defs[0].Errors[0].Code = "mutated"
				*defs[1].Request = gateway.Schema{}
				*defs[1].Response = gateway.Schema{}
			}
		}()
	}
	wg.Wait()
}

func TestSchemaPreservesNullablePointersCountsAndExactScalarCells(t *testing.T) {
	type evidence struct {
		Cells    []json.RawMessage `json:"cells"`
		Counts   map[string]int    `json:"counts"`
		Estimate *float64          `json:"estimate"`
		Finished *time.Time        `json:"finished"`
		Nested   *requestDTO       `json:"nested,omitempty"`
	}
	schema, err := SchemaFor("evidence", reflect.TypeFor[evidence](), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range []string{
		`{"cells":["9007199254740993.125",true,null,1.25],"counts":{"numeric":2},"estimate":null,"finished":null}`,
		`{"cells":null,"counts":null,"estimate":1.5,"finished":"2026-09-07T12:00:00Z","nested":{"name":"x","values":null}}`,
	} {
		if err := schema.Validate([]byte(wire), 65536); err != nil {
			t.Fatal("valid nullable evidence rejected", err)
		}
	}
	for _, wire := range []string{
		`{"cells":[{"arbitrary":"object"}],"counts":{},"estimate":null,"finished":null}`,
		`{"cells":[[]],"counts":{},"estimate":null,"finished":null}`,
		`{"cells":[],"counts":{"numeric":"two"},"estimate":null,"finished":null}`,
	} {
		if schema.Validate([]byte(wire), 65536) == nil {
			t.Fatal("unqualified evidence admitted", wire)
		}
	}
	if _, err := SchemaFor("input", reflect.TypeFor[evidence](), false); err == nil {
		t.Fatal("response-only types admitted in request")
	}
	if _, err := SchemaFor("bad", reflect.TypeFor[requestDTO](), true, OptionalJSONFields); err == nil {
		t.Fatal("request decoder option applied to response")
	}
	if _, err := SchemaFor("bad", reflect.TypeFor[requestDTO](), false, SchemaOption(99)); err == nil {
		t.Fatal("unknown schema option")
	}
	if _, err := SchemaFor("bad", reflect.TypeFor[requestDTO](), false, OptionalJSONFields, OptionalJSONFields); err == nil {
		t.Fatal("duplicate schema options")
	}
}

func TestBinaryRegistrationUsesItsActualContentTypeAndCap(t *testing.T) {
	d := definition(t)
	d.Method = "PUT"
	d.RequestContentType = "application/octet-stream"
	d.MaxBodyBytes = 100 << 20
	binary, err := gateway.NewSchema("binary", []byte(`{"type":"string","format":"binary"}`))
	if err != nil {
		t.Fatal(err)
	}
	d.Request = binary
	r, err := New([]Definition{d})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := r.OpenAPI("Upload", "1")
	if err != nil || !strings.Contains(string(wire), `"application/octet-stream"`) {
		t.Fatal("binary body metadata lost", err)
	}
	for _, edit := range []func(*Definition){func(d *Definition) { d.RequestContentType = "text/plain" }, func(d *Definition) { d.MaxBodyBytes++ }, func(d *Definition) { d.Method = "GET"; d.Request = nil; d.MaxBodyBytes = 0 }} {
		bad := d
		edit(&bad)
		if _, err := New([]Definition{bad}); err == nil {
			t.Fatal("invalid binary registration accepted")
		}
	}
}
