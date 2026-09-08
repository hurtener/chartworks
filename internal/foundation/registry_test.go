package foundation

import "testing"

func TestPublicRegistryAndBasePathContracts(t *testing.T) {
	registry, err := PublicRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definitions := registry.Definitions()
	if len(definitions) != 8 {
		t.Fatalf("public definitions=%d", len(definitions))
	}
	for _, definition := range definitions {
		if !definition.Public || definition.Action != "" || definition.Response == nil || len(definition.Errors) != 4 {
			t.Fatalf("incomplete public definition: %#v", definition)
		}
		if _, _, known := registry.Match("DELETE", definition.Path); !known {
			t.Fatalf("known public path lost method mismatch: %s", definition.Path)
		}
	}
	for _, test := range []struct {
		base, path, want string
		ok               bool
	}{
		{base: "/", path: "/healthz", want: "/healthz", ok: true},
		{base: "", path: "/healthz", want: "/healthz", ok: true},
		{base: "/api", path: "/api/healthz", want: "/healthz", ok: true},
		{base: "/api", path: "/api", want: "/", ok: true},
		{base: "/api", path: "/other/healthz", ok: false},
	} {
		got, ok := trimBasePath(test.base, test.path)
		if got != test.want || ok != test.ok {
			t.Fatalf("trimBasePath(%q, %q)=(%q,%t), want (%q,%t)", test.base, test.path, got, ok, test.want, test.ok)
		}
	}
}
