package chartworks

import "github.com/hurtener/chartworks/internal/reporting"

// QuerySelections preserves explicit bounded routing choices in a replayable
// query widget. It does not carry SQL, prompts, credentials or access grants.
type QuerySelections = reporting.QuerySelections
