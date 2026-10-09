package config

import "testing"

func TestReportingAppExplicitOriginRegistration(t *testing.T) {
	if DefaultReporting().App.EmbeddedEnabled {
		t.Fatal("embedded host enabled by default")
	}
	for _, c := range []ReportingApp{{}, {EmbeddedEnabled: true, RegisteredParentOrigins: []string{"https://app.example.test"}}, {RegisteredParentOrigins: []string{"https://app.example.test:8443"}}} {
		if err := c.Validate(); err != nil {
			t.Fatal(c, err)
		}
	}
	for _, origins := range [][]string{nil, {"*"}, {"null"}, {"https://*.example.test"}, {"https://app.example.test/path"}, {"https://user:secret@app.example.test"}, {"https://app.example.test?token=x"}, {"http://localhost:3000"}, {"https://app.example.test:443"}, {"https://app.example.test?"}, {"https://app.example.test", "https://app.example.test"}, make([]string, 17)} {
		if err := (ReportingApp{EmbeddedEnabled: true, RegisteredParentOrigins: origins}).Validate(); err == nil {
			t.Fatal("unregistered parent admitted", origins)
		}
	}
}
