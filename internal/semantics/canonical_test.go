package semantics

import (
	"slices"
	"testing"
)

func TestCanonicalMeaningExcludesLocalKeysAndNormalizesTerms(t *testing.T) {
	pack := testPack()
	model, err := Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	meanings := model.CanonicalMeanings()
	if len(meanings) != 1 || meanings[0].ID != "customer" || meanings[0].Revision != 2 {
		t.Fatalf("meaning projection: %#v", meanings)
	}
	wantDigest := meanings[0].Digest()
	if wantDigest != "5bf6d89e23fc398800dee5b5bca38bb8d72a888c4648a47f6a40392b081c5e42" || !slices.Equal(meanings[0].Terms(), []string{"customer", "buyer", "cliente"}) {
		t.Fatalf("meaning identity: %q %#v", wantDigest, meanings[0].Terms())
	}
	slices.Reverse(pack.CanonicalEntities[0].Keys)
	remapped, err := Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if remapped.CanonicalMeanings()[0].Digest() != wantDigest || remapped.Digest() == model.Digest() {
		t.Fatal("local key remap changed global meaning or failed to change topic meaning")
	}

	fullWidth := meanings[0]
	fullWidth.Name = "ＣＵＳＴＯＭＥＲ"
	if !slices.Equal(fullWidth.Terms()[:1], []string{"customer"}) {
		t.Fatal("NFKC/case-fold normalization changed")
	}
}
