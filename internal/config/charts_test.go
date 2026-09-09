package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChartsConfigurationRoundTripAndBounds(t *testing.T) {
	v := good()
	v.Charts.RankEnabled = true
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(bytes.NewReader(data), func(string) (string, bool) { return "fixture", true }, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Values().Charts != v.Charts || cfg.Values().Charts.ServiceOptions().RankEnabled != true {
		t.Fatal("chart config projection")
	}
	copy := cfg.Values()
	copy.Charts.Limits.MaxRows = 1
	if cfg.Values().Charts.Limits.MaxRows == 1 {
		t.Fatal("mutable config")
	}
	for _, edit := range []func(*Charts){func(c *Charts) { c.MaxConcurrent = 0 }, func(c *Charts) { c.Timeout = Duration(31 * time.Second) }, func(c *Charts) { c.RankCalls = 0 }, func(c *Charts) { c.RankTokens = 0 }, func(c *Charts) { c.RankTimeout = c.Timeout + 1 }, func(c *Charts) { c.Limits.MaxRows = 0 }, func(c *Charts) { c.Limits.MaxColumns = 0 }, func(c *Charts) { c.Limits.MaxBytes = 0 }, func(c *Charts) { c.Limits.MaxCellBytes = 0 }, func(c *Charts) { c.Limits.MaxCategories = 0 }, func(c *Charts) { c.Limits.MaxSeries = 0 }, func(c *Charts) { c.Limits.MaxAlternatives = 14 }, func(c *Charts) { c.Limits.SelectionFloor = 101 }, func(c *Charts) { c.Limits.MaxOptionsBytes = 0 }, func(c *Charts) { c.Limits.MaxOptionsDepth = 0 }} {
		bad := good()
		edit(&bad.Charts)
		if validate(bad) == nil {
			t.Fatal("chart bounds ignored")
		}
	}
	for _, wire := range []string{`{"charts":{"warehouse_url":"PRIVATE"}}`, `{"charts":{"limits":{"formatter":"PRIVATE"}}}`, `{"charts":{"rank_enabled":null}}`} {
		if _, err := Load(strings.NewReader(wire), nil, Overrides{}); err == nil || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("unclosed chart configuration", err)
		}
	}
}
