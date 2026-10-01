package sqlpolicy

import "strings"

// MySQLUTCInstantSyntax is shared generation/validation guidance for the one
// reviewed session-independent instant-to-civil form. Source authority and exact
// physical timestamp typing remain mandatory at the structural consumer.
const MySQLUTCInstantSyntax = "CAST(physical_timestamp AT TIME ZONE '+00:00' AS DATETIME(6)); UTC is the only alternative literal zone; no civil input, expression, or reduced precision"

func MySQLUTCZone(zone string) bool { return zone == "+00:00" || zone == "UTC" }

func MySQLTimestampType(native string) bool {
	native = strings.ToLower(strings.TrimSpace(native))
	if native == "timestamp" {
		return true
	}
	return len(native) == 12 && strings.HasPrefix(native, "timestamp(") && native[10] >= '0' && native[10] <= '6' && native[11] == ')'
}
