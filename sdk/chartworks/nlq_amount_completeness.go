package chartworks

import "github.com/hurtener/chartworks/internal/nlqexec"

// NLQAmountCompleteness is proof-derived missing-amount evidence for returned
// query rows. It does not replace result truncation or source coverage metadata.
type NLQAmountCompleteness = nlqexec.AmountCompleteness
type NLQAmountCompletenessRow = nlqexec.AmountCompletenessRow
