package securityapi

import "net/url"

func parseRawQuery(s string) (url.Values, error) { return url.ParseQuery(s) }
