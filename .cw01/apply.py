"""Apply the checked CW-01 lint remediation; do not modify runtime policy."""
from pathlib import Path
import re

pending = {}

def read(path):
    if path not in pending:
        pending[path] = Path(path).read_text()
    return pending[path]

def replace(path, before, after):
    text = read(path)
    if text.count(before) != 1:
        raise SystemExit(f"{path}: expected exactly one reviewed replacement")
    pending[path] = text.replace(before, after, 1)

def document(path, declaration, comment):
    replace(path, declaration, comment + "\n" + declaration)

# These six inspected import blocks contain ordinary, unaliased imports only.
for path in (
    "internal/nlqapi/clarification.go",
    "internal/nlqapi/clarification_test.go",
    "internal/nlqroute/clarification_groups.go",
    "internal/store/postgres/clarification_comparison.go",
    "sdk/chartworks/clarification.go",
    "sdk/chartworks/clarification_authoring.go",
):
    text = read(path)
    match = re.search(r"import \(\n(.*?)\n\)", text, re.S)
    if match is None:
        raise SystemExit(f"{path}: import block missing")
    imports = [line.strip() for line in match.group(1).splitlines() if line.strip()]
    if any(re.fullmatch(r'"[^"\n]+"', item) is None for item in imports):
        raise SystemExit(f"{path}: unexpected import syntax")
    standard = sorted(item for item in imports if "." not in item.strip('"').split('/')[0])
    external = sorted(item for item in imports if item not in standard)
    groups = ["\n".join("\t" + item for item in group) for group in (standard, external) if group]
    pending[path] = text[:match.start()] + "import (\n" + "\n\n".join(groups) + "\n)" + text[match.end():]

comments = {
    "internal/exec/business_model.go": [
        ("func (BusinessConstraint) String()", "// String excludes business scalar values from ordinary formatted logs."),
        ("func (c BusinessConstraint) GoString()", "// GoString preserves value redaction for Go-syntax formatting."),
    ],
    "internal/exec/business_sql.go": [
        ("type BusinessBoundQuery struct {", "// BusinessBoundQuery carries protected SQL and parameters pending ordinary read validation."),
        ("func (BusinessBoundQuery) String()", "// String omits SQL and parameter values from ordinary formatted logs."),
        ("func (b BusinessBoundQuery) GoString()", "// GoString preserves protected-query redaction for Go-syntax formatting."),
    ],
    "internal/nlqexec/clarification.go": [
        ("func (ClarificationEvidence) LogValue()", "// LogValue omits protected clarification evidence from ordinary structured logs."),
        ("func (v ClarificationEvidence) GoString()", "// GoString preserves evidence redaction for Go-syntax formatting."),
    ],
    "internal/semantics/clarification_problem.go": [
        ("func (ClarificationProblem) LogValue()", "// LogValue excludes repair payloads from ordinary structured logs."),
        ("func (ClarificationAnswer) LogValue()", "// LogValue excludes submitted answer values from ordinary structured logs."),
        ("func (ClarificationResolution) LogValue()", "// LogValue excludes resolved scalar values from ordinary structured logs."),
    ],
    "internal/semantics/clarification_runtime.go": [
        ("func (a ClarificationAnswer) GoString()", "// GoString preserves answer redaction for Go-syntax formatting."),
        ("func (r ClarificationResolution) GoString()", "// GoString preserves resolution redaction for Go-syntax formatting."),
    ],
    "internal/semantics/rulesets/clarification_authoring.go": [
        ("type ClarificationExportRequest struct {", "// ClarificationExportRequest selects an exact retained ruleset version."),
        ("type ClarificationImportPreview struct {", "// ClarificationImportPreview is a review-required proposal, never an activated policy."),
    ],
    "sdk/chartworks/clarification.go": [
        ("type ClarificationValue =", "// ClarificationValue preserves the service's closed typed-answer union."),
        ("type ClarificationTimeInput =", "// ClarificationTimeInput carries explicit calendar, timezone and interval inputs."),
        ("type ClarificationNumberInput =", "// ClarificationNumberInput preserves exact decimal text and the declared unit."),
        ("type ClarificationProblem =", "// ClarificationProblem carries a bounded localized repair response."),
        ("type ClarificationFieldError =", "// ClarificationFieldError identifies a field and a value-free repair message."),
    ],
    "sdk/chartworks/clarification_authoring.go": [
        ("type ClarificationInput =", "// ClarificationInput is a bounded synthetic clarification-preview case."),
        ("type ClarificationPreviewRequest =", "// ClarificationPreviewRequest pairs a draft definition with synthetic cases."),
        ("type ClarificationPreview =", "// ClarificationPreview contains deterministic effects and case outcomes."),
        ("type ClarificationExportRequest =", "// ClarificationExportRequest selects an exact reviewed ruleset version."),
        ("type PortableClarifications =", "// PortableClarifications preserves rule digests and migration dispositions."),
        ("type ClarificationImportRequest =", "// ClarificationImportRequest proposes an exact-topic portable-pack import."),
        ("type ClarificationImportPreview =", "// ClarificationImportPreview remains subject to ordinary review and publication."),
        ("func (c *Client) ExportClarifications(", "// ExportClarifications reads an exact retained pack under current export reach."),
        ("func (c *Client) PreviewClarificationImport(", "// PreviewClarificationImport validates a proposed import without publishing it."),
    ],
}
for path, entries in comments.items():
    for declaration, comment in entries:
        document(path, declaration, comment)
replace("sdk/chartworks/clarification.go",
    "// These aliases preserve the same versioned contract for HTTP, MCP and in-process\n// clients. Only the service can resolve them; they never confer read authority.",
    "// ClarificationAnswer preserves the versioned HTTP, MCP and in-process input.\n// Only the service can resolve it; it never confers read authority.")
replace("internal/semantics/clarification_types.go",
    "const (\n\tClarificationNotApplicable ClarificationOutcome",
    "// ClarificationNotApplicable, ClarificationSatisfied, ClarificationMissing,\n// ClarificationInvalid and ClarificationConflicting classify evaluated policy outcomes.\nconst (\n\tClarificationNotApplicable ClarificationOutcome")
replace("internal/semantics/clarification_values.go", "notación exponencial.", "notación con exponentes.")

replace("internal/exec/business_sql.go",
    '\t\t\tif c.TemporalType == "timestamp" {',
    '\t\t\tswitch c.TemporalType {\n\t\t\tcase "timestamp":')
replace("internal/exec/business_sql.go",
    '\t\t\t} else if c.TemporalType == "timestamptz" {',
    '\t\t\tcase "timestamptz":')
replace("internal/exec/business_sql.go",
    'i != from+1 && !(tokens[i-1].depth == 0 && (tokens[i-1].word("join") || tokens[i-1].text == ","))',
    'i != from+1 && (tokens[i-1].depth != 0 || !tokens[i-1].word("join") && tokens[i-1].text != ",")')

# All five helpers were reported unused by the exact-head lint job. Require
# exactly one identifier occurrence before deleting each complete definition.
path = "internal/nlqroute/service.go"
for name in ("appendUniqueRef", "choiceExists", "choicesFor", "topicVersions", "ruleVersions"):
    text = read(path)
    if len(re.findall(r"\b" + name + r"\b", text)) != 1:
        raise SystemExit(f"{path}: {name} acquired a consumer")
    pattern = re.compile(r"\nfunc " + name + r"\([^\n]*\{\n.*?\n\}\n?", re.S)
    matches = list(pattern.finditer(text))
    if len(matches) != 1:
        raise SystemExit(f"{path}: expected one complete {name} definition")
    pending[path] = text[:matches[0].start()] + "\n" + text[matches[0].end():]

# Validate every edit before writing any source. The existing scoped workflow
# formats these files, checks the staged diff, and commits only on this branch.
for path, text in sorted(pending.items()):
    if not path.startswith(("internal/", "sdk/")):
        raise SystemExit("unexpected edit target")
    Path(path).write_text(text)
print(f"Applied {len(pending)} inspected CW-01 lint remediations")
