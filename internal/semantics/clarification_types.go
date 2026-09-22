package semantics

// ClarificationSchemaVersion versions conditional policies and their resolution
// records independently of immutable topic and legacy ruleset encodings.
const ClarificationSchemaVersion = 1

// ClarificationWhen is a closed, reviewed OR of literal token phrases and exact
// semantic references. It cannot carry executable expressions or regular expressions.
type ClarificationWhen struct {
	AnyTerms      []string    `json:"any_terms,omitempty"`
	AnyReferences []Reference `json:"any_references,omitempty"`
}

// ClarificationPolicy controls applicability, not authority. A nil policy on a
// retained pattern is legacy reference-only behavior, never a new required blocker.
// Disabled is an explicit reviewed migration disposition, not a required-slot skip.
type ClarificationPolicy struct {
	SchemaVersion int               `json:"schema_version"`
	Disabled      bool              `json:"disabled,omitempty"`
	When          ClarificationWhen `json:"when"`
	Priority      int               `json:"priority"`
	Why           string            `json:"why"`
	WhySpanish    string            `json:"why_es,omitempty"`
}

// GovernedClarificationValue maps bounded spellings to one reviewed stored value.
// Canonical is a business value, not an option ID or a warehouse identifier.
type GovernedClarificationValue struct {
	Canonical string   `json:"canonical"`
	Label     string   `json:"label"`
	LabelES   string   `json:"label_es,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
}

// ClarificationEffect declares the meaning of a non-reference answer. Target is
// resolved through the topic's exact field graph, never through display labels.
// Numeric values use exact decimal strings. Bounds is one of [], [), (], ().
// Time windows are Gregorian, use an explicit IANA zone and always have [) bounds.
type ClarificationEffect struct {
	Kind         string                       `json:"kind"`
	Target       Reference                    `json:"target"`
	Operator     string                       `json:"operator"`
	Nulls        string                       `json:"nulls"`
	Unit         string                       `json:"unit,omitempty"`
	Precision    int                          `json:"precision,omitempty"`
	Scale        int                          `json:"scale,omitempty"`
	Bounds       string                       `json:"bounds,omitempty"`
	Calendar     string                       `json:"calendar,omitempty"`
	TimeZone     string                       `json:"time_zone,omitempty"`
	TemporalType string                       `json:"temporal_type,omitempty"`
	Grains       []string                     `json:"grains,omitempty"`
	MaxLength    int                          `json:"max_length,omitempty"`
	Values       []GovernedClarificationValue `json:"values,omitempty"`
}

// ClarificationTimeInput accepts either an explicit [start,end) date window or
// a single localized month period, never both. Zone, calendar and grain are
// explicit and must agree with the reviewed policy.
type ClarificationTimeInput struct {
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
	Period   string `json:"period,omitempty"`
	Calendar string `json:"calendar"`
	TimeZone string `json:"time_zone"`
	Grain    string `json:"grain"`
}

// ClarificationNumberInput preserves precision across JSON and client runtimes.
// Upper is present only for a reviewed range. Unit is an exact governed unit.
type ClarificationNumberInput struct {
	Value string `json:"value"`
	Upper string `json:"upper,omitempty"`
	Unit  string `json:"unit"`
}

// ClarificationValue is a bounded tagged union. Exactly one member is required.
// OptionID is reserved for exact reviewed reference choices; typed scalar values
// never pass through the legacy raw-string choice channel.
type ClarificationValue struct {
	OptionID string                    `json:"option_id,omitempty"`
	Time     *ClarificationTimeInput   `json:"time,omitempty"`
	Number   *ClarificationNumberInput `json:"number,omitempty"`
	Boolean  *string                   `json:"boolean,omitempty"`
	Text     *string                   `json:"text,omitempty"`
	Null     bool                      `json:"null,omitempty"`
}

// ClarificationAnswer carries caller input and exact publication pins. The
// service owns resolution and sensitivity classification. Remove is permitted
// only by the same-session refinement merge, never as a required-blocker skip.
type ClarificationAnswer struct {
	Topic          string              `json:"topic"`
	TopicVersion   string              `json:"topic_version"`
	RulesetVersion string              `json:"ruleset_version"`
	Pattern        string              `json:"pattern"`
	PatternVersion string              `json:"pattern_version"`
	Slot           string              `json:"slot"`
	Value          *ClarificationValue `json:"value,omitempty"`
	Remove         bool                `json:"remove,omitempty"`
}

// ClarificationOutcome distinguishes activation and answer states. Unresolved
// governed text is Missing with an explicit unresolved_value field error.
type ClarificationOutcome string

// ClarificationNotApplicable, ClarificationSatisfied, ClarificationMissing,
// ClarificationInvalid and ClarificationConflicting classify evaluated policy outcomes.
const (
	ClarificationNotApplicable ClarificationOutcome = "not_applicable"
	ClarificationSatisfied     ClarificationOutcome = "satisfied"
	ClarificationMissing       ClarificationOutcome = "missing"
	ClarificationInvalid       ClarificationOutcome = "invalid"
	ClarificationConflicting   ClarificationOutcome = "conflicting"
)

// ClarificationFieldError contains a localized, content-free correction hint.
// It never includes the supplied value, bearer, SQL, or provider prompt.
type ClarificationFieldError struct {
	Topic   string `json:"topic,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Slot    string `json:"slot,omitempty"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CanonicalClarificationTime retains the local and UTC [) boundaries so replay
// does not silently reinterpret a date under a different locale or timezone.
type CanonicalClarificationTime struct {
	BoundaryPolicy string `json:"boundary_policy"`
	StartUTC       string `json:"start_utc"`
	EndUTC         string `json:"end_utc"`
	LocalStart     string `json:"local_start"`
	LocalEnd       string `json:"local_end"`
	Calendar       string `json:"calendar"`
	TimeZone       string `json:"time_zone"`
	Grain          string `json:"grain"`
	Bounds         string `json:"bounds"`
}

// ClarificationResolution is protected query evidence, not a caller-issued
// proof. The route seals the question and publication pins; execution still
// requires the ordinary source-bound validator-issued read plan.
type ClarificationResolution struct {
	ParserVersion  string                      `json:"parser_version"`
	Locale         string                      `json:"locale"`
	SchemaVersion  int                         `json:"schema_version"`
	ID             string                      `json:"id"`
	Topic          string                      `json:"topic"`
	TopicVersion   string                      `json:"topic_version"`
	PackDigest     string                      `json:"pack_digest"`
	RulesetVersion string                      `json:"ruleset_version"`
	RulesetDigest  string                      `json:"ruleset_digest"`
	Pattern        string                      `json:"pattern"`
	PatternVersion string                      `json:"pattern_version"`
	Slot           string                      `json:"slot"`
	QuestionDigest string                      `json:"question_digest"`
	Provenance     string                      `json:"provenance"`
	Sensitivity    LiteralSensitivity          `json:"sensitivity"`
	Reference      *Reference                  `json:"reference,omitempty"`
	Effect         *ClarificationEffect        `json:"effect,omitempty"`
	Time           *CanonicalClarificationTime `json:"time,omitempty"`
	Value          string                      `json:"value,omitempty"`
	Upper          string                      `json:"upper,omitempty"`
	Null           bool                        `json:"null,omitempty"`
}

// ClarificationSlotOutcome is the user-facing explanation of one slot. Effects
// contain reviewed metadata only. Values remain in protected resolution records.
type ClarificationSlotOutcome struct {
	Specificity    int                       `json:"specificity"`
	Priority       int                       `json:"priority"`
	Order          int                       `json:"order"`
	Topic          string                    `json:"topic"`
	TopicVersion   string                    `json:"topic_version"`
	RulesetVersion string                    `json:"ruleset_version"`
	Pattern        string                    `json:"pattern"`
	PatternVersion string                    `json:"pattern_version"`
	Slot           string                    `json:"slot"`
	Outcome        ClarificationOutcome      `json:"outcome"`
	Reason         string                    `json:"reason"`
	Prompt         string                    `json:"prompt,omitempty"`
	Why            string                    `json:"why,omitempty"`
	Required       bool                      `json:"required"`
	Kind           SlotKind                  `json:"kind"`
	Choices        []ClarificationChoice     `json:"choices,omitempty"`
	Effect         *ClarificationEffect      `json:"effect,omitempty"`
	DependsOn      []string                  `json:"depends_on,omitempty"`
	Defaulted      bool                      `json:"defaulted,omitempty"`
	Errors         []ClarificationFieldError `json:"errors,omitempty"`
}

// ClarificationEvaluation is atomic with respect to invalid/conflicting input:
// neither outcome returns partially accepted resolutions or reference choices.
type ClarificationEvaluation struct {
	SchemaVersion int                        `json:"schema_version"`
	Outcome       ClarificationOutcome       `json:"outcome"`
	Slots         []ClarificationSlotOutcome `json:"slots"`
	Resolutions   []ClarificationResolution  `json:"resolutions,omitempty"`
	References    []Reference                `json:"references,omitempty"`
	Errors        []ClarificationFieldError  `json:"errors,omitempty"`
	Dispositions  []string                   `json:"dispositions,omitempty"`
}
