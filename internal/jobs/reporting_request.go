package jobs

// validDispatchedRequest preserves the accepted queue manifest across both
// production target families. Neither a session label nor a request body can
// convert ordinary request work into broker-authorized queued work.
func validDispatchedRequest(t RequestTask) bool {
	j := t.Dispatch
	if j == nil || !j.Valid() || t.ID != j.ID || t.Tenant != j.Tenant || t.Actor != j.Executor ||
		t.Session != j.ID || t.MaxAttempts != j.MaxAttempts || t.ManifestHash != j.ManifestHash ||
		t.Created.IsZero() || !t.Expires.After(t.Created) || t.ManifestHash != t.Digest() {
		return false
	}
	if j.Kind == PipelineKind {
		return t.Input == (RequestInput{Kind: PipelineKind, Target: j.Pipeline.ID, InputHash: j.Pipeline.Digest})
	}
	return j.Kind == ReportingKind && j.Reporting != nil && t.Input == j.Reporting.Input
}
