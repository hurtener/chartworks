package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/identity"
)

// RuleModel holds a detached, version-pinned authoring definition. Successful
// compilation is neither activation nor execution-constraint enforcement.
type RuleModel struct {
	definition RuleSetDefinition
	digest     string
}

// RuleSubject is the public semantic reference graph needed to compile rules.
// Its digest is the reviewed topic-pack digest; source profile evidence remains
// private to topic authoring and is neither reconstructed nor exposed here.
type RuleSubject struct {
	topic, version, digest string
	refs                   map[string]struct{}
	graph                  map[Reference][]Reference
}

// NewRuleSubject validates the retained public topic projection used by rules.
func NewRuleSubject(pack TopicPack, digest string) (RuleSubject, error) {
	if !identity.Identifier(pack.Topic) || !identity.Identifier(pack.Version) || len(digest) != 64 {
		return RuleSubject{}, invalid(CodeEvidenceMismatch, "ruleset.pack")
	}
	if _, err := hex.DecodeString(digest); err != nil || strings.ToLower(digest) != digest {
		return RuleSubject{}, invalid(CodeEvidenceMismatch, "ruleset.pack")
	}
	refs, err := referenceIndex(pack)
	if err != nil {
		return RuleSubject{}, err
	}
	if err = validateReferences(pack, refs); err != nil {
		return RuleSubject{}, err
	}
	return RuleSubject{topic: pack.Topic, version: pack.Version, digest: digest, refs: refs, graph: dependencyGraphPack(pack)}, nil
}

func (m RuleModel) Definition() RuleSetDefinition { return cloneRules(m.definition) }
func (m RuleModel) Digest() string                { return m.digest }

// CompileRules binds authoring rules and slots to an existing semantic model,
// rejecting stale references and contradictory mandatory dependency requirements.
// Priority never suppresses a hard constraint to hide a contradiction.
func CompileRules(model Model, input RuleSetDefinition) (RuleModel, error) {
	if model.Digest() == "" {
		return RuleModel{}, invalid(CodeEvidenceMismatch, "ruleset.pack")
	}
	return compileRules(RuleSubject{topic: model.pack.Topic, version: model.pack.Version, digest: model.Digest(), refs: model.refs, graph: dependencyGraphPack(model.pack)}, input)
}

// CompilePublishedRules binds rules to an immutable published topic projection.
func CompilePublishedRules(subject RuleSubject, input RuleSetDefinition) (RuleModel, error) {
	return compileRules(subject, input)
}

func compileRules(subject RuleSubject, input RuleSetDefinition) (RuleModel, error) {
	if subject.digest == "" || input.Topic != subject.topic || input.TopicVersion != subject.version || input.PackDigest != subject.digest {
		return RuleModel{}, invalid(CodeEvidenceMismatch, "ruleset.pack")
	}
	if err := validateRuleShape(input); err != nil {
		return RuleModel{}, err
	}
	p := cloneRules(input)
	sort.Slice(p.Rules, func(i, j int) bool { return p.Rules[i].ID < p.Rules[j].ID })
	sort.Slice(p.Patterns, func(i, j int) bool { return p.Patterns[i].ID < p.Patterns[j].ID })
	for i := range p.Rules {
		sortReferences(p.Rules[i].Scope.Targets)
	}
	for i := range p.Patterns {
		sortReferences(p.Patterns[i].Targets)
	}
	if err := validateRuleReferences(subject, p); err != nil {
		return RuleModel{}, err
	}
	if err := ruleConflicts(subject, p); err != nil {
		return RuleModel{}, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return RuleModel{}, invalid(CodeInvalidValue, "ruleset")
	}
	if len(raw) > maximumPackBytes {
		return RuleModel{}, invalid(CodeLimit, "ruleset")
	}
	sum := sha256.Sum256(raw)
	return RuleModel{definition: p, digest: hex.EncodeToString(sum[:])}, nil
}

func validateRuleShape(p RuleSetDefinition) error {
	if p.SchemaVersion != SchemaVersion || !identity.Identifier(p.ID) || !identity.Identifier(p.Version) {
		return invalid(CodeInvalidValue, "ruleset")
	}
	if len(p.Rules) > 256 || len(p.Patterns) > 128 {
		return invalid(CodeLimit, "ruleset")
	}
	for i, rule := range p.Rules {
		path := "rules[" + itoa(i) + "]"
		if !identity.Identifier(rule.ID) || !identity.Identifier(rule.Version) || !validProvenance(rule.Provenance) || rule.Priority < -1000 || rule.Priority > 1000 {
			return invalid(CodeInvalidValue, path)
		}
		switch rule.Category {
		case RuleComputation, RuleSemantic, RuleStructural:
		default:
			return invalid(CodeInvalidValue, path+".category")
		}
		if rule.Scope.Kind == RuleScopeTopic {
			if len(rule.Scope.Targets) != 0 {
				return invalid(CodeInvalidValue, path+".scope")
			}
		} else if rule.Scope.Kind != RuleScopeEntities || len(rule.Scope.Targets) < 1 || len(rule.Scope.Targets) > 32 {
			return invalid(CodeInvalidValue, path+".scope")
		}
		// These targets are sorted before reference lookup; bound their
		// coordinates here so sorting never builds keys from unbounded input.
		for _, ref := range rule.Scope.Targets {
			if !ref.Valid() {
				return invalid(CodeInvalidReference, path+".scope.targets")
			}
		}
		switch rule.Class {
		case RuleExecutionConstraint:
			if rule.Constraint == nil || rule.Guidance != nil {
				return invalid(CodeInvalidValue, path)
			}
			if rule.Constraint.Kind != ConstraintRequireReference && rule.Constraint.Kind != ConstraintExcludeReference {
				return invalid(CodeInvalidValue, path+".constraint.kind")
			}
		case RuleAdvisoryContext:
			if rule.Constraint != nil || rule.Guidance == nil || !validText(rule.Guidance.Text, 4096) || strings.TrimSpace(rule.Guidance.Text) == "" || !validSensitivity(rule.Guidance.Sensitivity) {
				return invalid(CodeInvalidValue, path+".guidance")
			}
		default:
			return invalid(CodeInvalidValue, path+".class")
		}
	}
	for i, pattern := range p.Patterns {
		path := "patterns[" + itoa(i) + "]"
		if !identity.Identifier(pattern.ID) || !identity.Identifier(pattern.Version) || !validProvenance(pattern.Provenance) || len(pattern.Targets) < 1 || len(pattern.Targets) > 32 || len(pattern.Slots) < 1 || len(pattern.Slots) > 16 {
			return invalid(CodeInvalidValue, path)
		}
		for _, ref := range pattern.Targets {
			if !ref.Valid() {
				return invalid(CodeInvalidReference, path+".targets")
			}
		}
		for j, slot := range pattern.Slots {
			path := path + ".slots[" + itoa(j) + "]"
			if !identity.Identifier(slot.ID) || !validLine(slot.Prompt, 1024) || !validSensitivity(slot.Sensitivity) {
				return invalid(CodeInvalidValue, path)
			}
			switch slot.Kind {
			case SlotChoice:
				if len(slot.Choices) < 2 || len(slot.Choices) > 32 {
					return invalid(CodeLimit, path+".choices")
				}
			case SlotText, SlotNumber, SlotBoolean, SlotDate:
				if len(slot.Choices) != 0 {
					return invalid(CodeInvalidValue, path+".choices")
				}
			default:
				return invalid(CodeInvalidValue, path+".kind")
			}
			for k, choice := range slot.Choices {
				if !identity.Identifier(choice.ID) || !validLine(choice.Label, 256) {
					return invalid(CodeInvalidValue, path+".choices["+itoa(k)+"]")
				}
			}
		}
	}
	return nil
}

func validProvenance(p RuleProvenance) bool {
	if !identity.Identifier(p.Evidence) {
		return false
	}
	switch p.Kind {
	case ProvenanceHuman, ProvenanceModel, ProvenanceFeedback, ProvenanceImport:
		return true
	}
	return false
}

func validSensitivity(s LiteralSensitivity) bool {
	return s == LiteralNonSensitive || s == LiteralSensitive
}

func validateRuleReferences(subject RuleSubject, p RuleSetDefinition) error {
	checkTargets := func(refs []Reference, path string) error {
		seen := map[Reference]bool{}
		for _, ref := range refs {
			if !ref.Valid() || seen[ref] {
				return invalid(CodeInvalidReference, path)
			}
			if _, ok := subject.refs[ref.key()]; !ok {
				return invalid(CodeMissingReference, path)
			}
			seen[ref] = true
		}
		return nil
	}
	ids := map[string]bool{}
	for i, rule := range p.Rules {
		path := "rules[" + itoa(i) + "]"
		if ids[rule.ID] {
			return invalid(CodeDuplicateID, path+".id")
		}
		ids[rule.ID] = true
		if err := checkTargets(rule.Scope.Targets, path+".scope.targets"); err != nil {
			return err
		}
		if rule.Constraint != nil {
			ref := rule.Constraint.Target
			if ref.Kind == KindCanonicalEntity {
				// Registry keys may cover several datasets. They are identity
				// metadata, not one executable dependency requirement.
				return invalid(CodeInvalidReference, path+".constraint.target")
			}
			if err := checkTargets([]Reference{ref}, path+".constraint.target"); err != nil {
				return err
			}
			if rule.Scope.Kind == RuleScopeEntities && !hasReference(rule.Scope.Targets, ref) {
				return invalid(CodeInvalidReference, path+".constraint.target")
			}
		}
	}
	ids = map[string]bool{}
	for i, pattern := range p.Patterns {
		path := "patterns[" + itoa(i) + "]"
		if ids[pattern.ID] {
			return invalid(CodeDuplicateID, path+".id")
		}
		ids[pattern.ID] = true
		if err := checkTargets(pattern.Targets, path+".targets"); err != nil {
			return err
		}
		slots := map[string]bool{}
		for j, slot := range pattern.Slots {
			path := path + ".slots[" + itoa(j) + "]"
			if slots[slot.ID] {
				return invalid(CodeDuplicateID, path+".id")
			}
			slots[slot.ID] = true
			choices := map[string]bool{}
			for k, choice := range slot.Choices {
				path := path + ".choices[" + itoa(k) + "]"
				if choices[choice.ID] {
					return invalid(CodeDuplicateID, path+".id")
				}
				choices[choice.ID] = true
				if choice.Target != nil {
					if err := checkTargets([]Reference{*choice.Target}, path+".target"); err != nil {
						return err
					}
					if !hasReference(pattern.Targets, *choice.Target) {
						return invalid(CodeInvalidReference, path+".target")
					}
				}
			}
		}
	}
	return nil
}

func hasReference(refs []Reference, target Reference) bool {
	for _, ref := range refs {
		if ref == target {
			return true
		}
	}
	return false
}

// dependencyGraph deliberately models only the existing exact reference graph.
// It never infers joins, parses KPI expression text, or treats column names as IDs.
func dependencyGraphPack(pack TopicPack) map[Reference][]Reference {
	graph := map[Reference][]Reference{}
	for _, dataset := range pack.Datasets {
		for _, column := range dataset.Columns {
			graph[Reference{Kind: KindColumn, Dataset: dataset.ID, ID: column.ID}] = []Reference{{Kind: KindDataset, ID: dataset.ID}}
		}
	}
	for _, measure := range pack.Measures {
		graph[Reference{Kind: KindMeasure, ID: measure.ID}] = []Reference{measure.Field}
	}
	for _, dimension := range pack.Dimensions {
		graph[Reference{Kind: KindDimension, ID: dimension.ID}] = []Reference{dimension.Field}
	}
	for _, kpi := range pack.KPIs {
		graph[Reference{Kind: KindKPI, ID: kpi.ID}] = kpi.Inputs
	}
	for _, join := range pack.Joins {
		graph[Reference{Kind: KindJoin, ID: join.ID}] = []Reference{join.Left, join.Right}
	}
	return graph
}

func ruleConflicts(subject RuleSubject, p RuleSetDefinition) error {
	excluded := map[Reference]string{}
	for _, rule := range p.Rules {
		if rule.Constraint != nil && rule.Constraint.Kind == ConstraintExcludeReference {
			if _, exists := excluded[rule.Constraint.Target]; !exists {
				excluded[rule.Constraint.Target] = rule.ID
			}
		}
	}
	graph := subject.graph
	for _, rule := range p.Rules {
		if rule.Constraint == nil || rule.Constraint.Kind != ConstraintRequireReference {
			continue
		}
		seen := map[Reference]bool{}
		pending := []Reference{rule.Constraint.Target}
		for len(pending) > 0 {
			ref := pending[0]
			pending = pending[1:]
			if seen[ref] {
				continue
			}
			seen[ref] = true
			if blockedBy, exists := excluded[ref]; exists {
				return &RuleConflictError{RequiredBy: rule.ID, ExcludedBy: blockedBy, Target: ref}
			}
			pending = append(pending, graph[ref]...)
		}
	}
	return nil
}

func sortReferences(refs []Reference) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].key() < refs[j].key() })
}

func cloneRules(p RuleSetDefinition) RuleSetDefinition {
	p.Rules = append([]RuleDefinition(nil), p.Rules...)
	for i := range p.Rules {
		rule := &p.Rules[i]
		rule.Scope.Targets = append([]Reference(nil), rule.Scope.Targets...)
		if rule.Constraint != nil {
			copy := *rule.Constraint
			rule.Constraint = &copy
		}
		if rule.Guidance != nil {
			copy := *rule.Guidance
			rule.Guidance = &copy
		}
	}
	p.Patterns = append([]ClarificationPattern(nil), p.Patterns...)
	for i := range p.Patterns {
		pattern := &p.Patterns[i]
		pattern.Targets = append([]Reference(nil), pattern.Targets...)
		pattern.Slots = append([]ClarificationSlot(nil), pattern.Slots...)
		for j := range pattern.Slots {
			slot := &pattern.Slots[j]
			slot.Choices = append([]ClarificationChoice(nil), slot.Choices...)
			for k := range slot.Choices {
				if slot.Choices[k].Target != nil {
					copy := *slot.Choices[k].Target
					slot.Choices[k].Target = &copy
				}
			}
		}
	}
	return p
}
