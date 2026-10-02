# Explicit current-semantic example requalification

`POST /v1/nlq/examples/requalify`, MCP `requalify_example`, and the SDK
`RequalifyExampleNLQ` accept an exact retained example ID/version and a current
question anchor. Feedback, query-planning, SQL inspection and all current topic,
source, dataset/context permissions remain required. Routing can incur bounded
model cost; the operation never generates replacement SQL or executes rows.

The existing value-free SQL/template is revalidated natively, including typed
parameter domains. Current reviewed metric, population, grain, ordering and limit
proof must also pass. Merely executable old SQL is insufficient: an old SUM cannot
be requalified against a reviewed AVG definition. Current owned predicates are
bound only for this proof; their private values never enter the new example.

Success creates a separate candidate, never changes or activates the original.
Its immutable origin records the previous example ID/version/content digest and
origin digest, plus the current analytical-contract digest. Historical evidence
counts remain identified by that earlier origin; they are not new result-quality
observations. A distinct ordinary activation review is still required. Only then
can current-origin retrieval select the example for a rendered prompt.

Insertion atomically checks the retained version/origin and current publication,
source revisions and rule head. Exact retries return the same row without adding
evidence, even after separate activation. The change is audited. Protected
portable version 5 retains both origins; import repeats this same fresh proof and
requires the exact locally retained parent rather than treating an imported
contract hash as authority. Older portable versions are unchanged.

Scope: compatible existing SQL/templates and a measured current analytical
contract. Requalification does not repair semantically changed SQL, invent a new
metric, transfer historical private values or approve a topic publication. Those
cases require a newly reviewed intent/query and ordinary feedback workflow.

An unmeasured list-query example cannot acquire a semantic-correctness claim from
native executability alone. Likewise, a typed template whose public validation
probes do not establish the current reviewed constants is refused; historical
parameter values are not recovered to make it pass. The caller can instead create
and review a new current query. These are explicit qualification limits, not
claims that every old example can be automatically migrated.
