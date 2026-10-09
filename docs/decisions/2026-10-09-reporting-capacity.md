# Team report capacity

### D-105 — Bounded exact authority for team reports

Accepted implementation scope, 2026-10-09. Supersedes only the numerical
provider-token scope ceilings in D-059–D-061. Claim spelling, issuer ownership,
exact resource enforcement, privacy and temporal checks remain unchanged.

The first manual integration qualified five independent private chart outputs;
the sixth exceeded the 32-entry authority ceiling before work. Splitting pages
does not reduce a report's dependency closure. A usable team report needs a
larger bounded exact list, not a wildcard or a second authority mechanism.

Pengui's existing provider mint and Chartworks's verifier/envelope admit at most
128 unique printable ASCII scopes, 256 bytes each and 16,384 aggregate string
bytes. Chartworks defaults adopt those ceilings; explicit configuration can
retain smaller bounds. Existing token/claim ceilings stay 32,768/24,576 bytes.
No claim aliases, compressed permissions, locally signed references or new
identity tables are introduced. Each granted ID and permission remains explicit.

The App projection reserves one entry and seven bytes for `mcp.use` even on HTTP,
so domain projection has the same 127-entry/16,377-byte maximum on both paths.
All actions and complete native dependencies still pass canonical Pengui policy
and final session/generation checks. Too-large sets fail before admission with
the existing recoverable capacity error; no partial list can be used. Static
operator-configured capability scope lists retain their independent smaller
configuration limits. Metadata manifests remain bounded independently.

The bundled manual App and generic static MCP resource admission have a 512 KiB
ceiling. The small read viewer retains its 256 KiB ceiling. Build verification
reserves 64 KiB of the App budget for document framing and growth. Runtime code
remains one immutable inline bundle with hash CSP, no remote imports, network,
credentials or tenant data. Tool/data response budgets remain independent.

Rollout requires matching Pengui and Chartworks binaries. A legacy verifier or
explicit 32-entry configuration safely rejects the larger token; it is not
silently bypassed. Install the consumer before enabling larger authoring flows.
No schema migration is required. Local boundary tests do not replace the
twelve-chart/two-page HTTP/MCP acceptance in the
[team-ready plan](../plans/team-ready-reporting.md), nor authorize deployment.
