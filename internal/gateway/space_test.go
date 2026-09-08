package gateway

import "testing"

func TestEmbeddingSpaceKeyBindsCompleteDescriptor(t *testing.T) {
	base := EmbeddingSpace{Provider: "openrouter", Route: "primary", Endpoint: "https://gateway.example.test/v1", Model: "embed", Revision: "r1", Dimensions: 2, Preprocessing: "utf8-exact;float32-finite", InputType: "text", Normalization: "no-normalization"}
	key := base.Key()
	const golden = "e1ab7dbdeafcd3b41a050be1bae4250a9ea843ae0e2077afafc86f1597837bba"
	if key != golden || key != base.Key() {
		t.Fatal("unstable embedding space key")
	}
	variants := []EmbeddingSpace{base, base, base, base, base, base, base, base, base}
	variants[0].Provider = "other"
	variants[1].Route = "other"
	variants[2].Endpoint = "https://other.example.test/v1"
	variants[3].Model = "other"
	variants[4].Revision = "r2"
	variants[5].Dimensions = 3
	variants[6].Preprocessing = "other"
	variants[7].InputType = "query"
	variants[8].Normalization = "unit"
	for i, variant := range variants {
		if variant.Key() == key {
			t.Fatal("descriptor field omitted from key", i)
		}
	}
}
