package querydiagnostic

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestSQLRecoveryDiagnosticExactStates(t *testing.T) {
	expected := map[string]string{"22012": "query_division_by_zero", "22003": "query_numeric_range", "22P02": "query_invalid_text", "22018": "query_invalid_text", "22007": "query_invalid_datetime", "22008": "query_datetime_range", "21000": "query_cardinality", "42883": "query_function_signature", "42725": "query_function_signature", "42804": "query_type_mismatch", "42P18": "query_type_mismatch", "42803": "query_grouping", "42P20": "query_windowing"}
	for state, want := range expected {
		if got := Postgres(state); got != want || !Known(got) || Hint(got) == "" {
			t.Fatal("diagnostic mapping", state, got)
		}
	}
	for _, state := range []string{"", "22", "42", "22p02", "22012 ", " 22012", "220120", "42501", "42P01", "42703", "57014", "25P03", "25P04", "40001", "40P01", "08006", "53200", "53300", "53400", "57P01", "22000", "22023", "private division by zero 22012"} {
		if Postgres(state) != "" {
			t.Fatal("non-reviewed/terminal state became query repair", state)
		}
	}
}
func TestSQLRecoveryDiagnosticClosedDetachedHints(t *testing.T) {
	expected := []string{"query_error", "query_division_by_zero", "query_numeric_range", "query_invalid_text", "query_invalid_datetime", "query_datetime_range", "query_cardinality", "query_function_signature", "query_type_mismatch", "query_grouping", "query_windowing"}
	if !reflect.DeepEqual(Codes(), expected) {
		t.Fatal("unreviewed diagnostic set")
	}
	first := Codes()
	first[0] = "changed"
	if !reflect.DeepEqual(Codes(), expected) {
		t.Fatal("registry aliases caller memory")
	}
	for _, code := range expected {
		hint := Hint(code)
		if hint == "" || len(hint) > 512 || !utf8.ValidString(hint) || strings.ContainsAny(hint, "\x00\r\n") {
			t.Fatal("unbounded hint")
		}
	}
	for _, code := range []string{"", "query_error: private", "private-canary", "22012", "QUERY_ERROR", "query_error\x00"} {
		if Known(code) || Hint(code) != "" {
			t.Fatal("untrusted diagnostic echoed", code)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if Postgres("22012") != "query_division_by_zero" || Hint("query_error") == "" {
				t.Error("concurrent registry drift")
			}
		}()
	}
	wg.Wait()
}
func FuzzSQLRecoveryDiagnosticVocabulary(f *testing.F) {
	for _, value := range []string{"", "22012", "query_error", "42P01", "private-canary", string([]byte{0xff})} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, input string) {
		code := Postgres(input)
		if code != "" && (!Known(code) || Hint(code) == "") {
			t.Fatal("inconsistent state")
		}
		hint := Hint(input)
		if (hint != "") != Known(input) || len(hint) > 512 || !utf8.ValidString(hint) {
			t.Fatal("unbounded/unreviewed hint")
		}
	})
}
