package reportingapi

import (
	"strings"
	"testing"
)

func TestReportingRegistryFeatureSets(t *testing.T) {
	for _, validation := range []bool{false, true} {
		for _, capture := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				registry, err := Registry(validation, capture, observe)
				if err != nil || registry == nil {
					t.Fatalf("features validate=%v capture=%v observe=%v: %v", validation, capture, observe, err)
				}
			}
		}
	}
}

func TestDeliveryRegistryAdvertisesStaticExportOnlyWhenMounted(t *testing.T) {
	without, err := DeliveryRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range without.Definitions() {
		if definition.ID == "reportingExport" {
			t.Fatal("unmounted exporter advertised")
		}
	}
	with, err := DeliveryRegistry(false, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, definition := range with.Definitions() {
		if definition.ID == "reportingExport" {
			found = definition.Action == "reporting.export" && definition.Effect == "retained_static_rendition" && definition.Request != nil
			body := definition.Request.Document()
			if !strings.Contains(string(body), `"additionalProperties":false`) || strings.Contains(string(body), `"url"`) || strings.Contains(string(body), `"script"`) {
				t.Fatal("open or executable export schema", string(body))
			}
		}
	}
	if !found {
		t.Fatal("static exporter not registered with exact authority/effect")
	}
}

func TestFilterOptionsAdvertisedOnlyWithValidatedExecution(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		registry, err := DeliveryRegistry(enabled)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, definition := range registry.Definitions() {
			if definition.ID != "reportingFilterOptions" {
				continue
			}
			found = definition.Action == "reporting.execute" && definition.Effect == "bounded_validated_distinct_source_read" && definition.Request != nil && definition.Response != nil
		}
		if found != enabled {
			t.Fatal("filter option capability registration mismatch", enabled, found)
		}
	}
}
