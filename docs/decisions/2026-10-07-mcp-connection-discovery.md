# MCP service connection discovery

### D-104 — Consume Pengui service connection authority for static MCP discovery

Accepted as an implementation correction within the authorized Builder/Consumer
integration, 2026-10-07. Owners: phases22/31, internal/mcpserver.

The real activation path uses Pengui's established `MintCapabilityConnection`
bearer, whose sole scope is `capability:connect`. Treating it as an interactive
caller prevented initialization. Replacing it with all domain actions would
misrepresent its purpose.

Chartworks consumes that exact scope only on a verified service identity and
configured MCP audience. It permits initialize/initialized/ping and listing static
tool/resource/template descriptors for the installed enabled groups. It denies
tools/call, every resources/read (even static HTML), and all other methods before
domain dispatch. Optional prompts/list returns standard JSON-RPC method-not-found
without advertising a prompt capability. Interactive requests still need `mcp.use` and ordinary exact
authority. Mixed profiles without `mcp.use` do not enter discovery. The existing
Pengui issuer and canonical service mint remain unchanged.

This narrows the former blanket mcp.use requirement solely for transport discovery
in mcp-v1; it does not change resource policy, users, grants, maintenance execution
or token renewal. Registry descriptors contain schemas and static descriptions,
not tenant data. No persistence or migration is needed.

Verification: TestPenguiConnectionDiscoveryCannotInvokeOrRead exercises real signed
HTTP initialization/discovery, no-action service discovery, every read/call denial,
human/mixed/audience/expired rejection and zero domain calls. Phase22 AC01/AC05
own the seam. Real activation and both-mode UI qualification remain separately
reported in the integration review.
