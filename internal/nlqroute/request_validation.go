package nlqroute

// ValidateRequest applies the same bounded input contract used by Route without
// reading publications, looking up credentials, or invoking a model. Saved
// consumers use this before persisting routing selections; it grants no reach.
func ValidateRequest(in RouteRequest) error {
	_, err := normalizeRequest(in)
	return err
}
