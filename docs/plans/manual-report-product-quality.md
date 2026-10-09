# Manual report product quality

Status: locally qualified, 2026-10-08. This continuation belongs to phases 29/31 and the
existing manual application. It does not close the whole-product release gate.

## Starting point

The real local HTTP and MCP journeys are qualified at the integration boundary
in [the two-mode record](../reviews/manual-app-two-mode-2026-10-07.md). Central
authority, immutable publication, private previews, source-backed preparation,
retained reads and host lifecycle fencing remain the baseline.

This continuation addressed: compact host composition, useful library
navigation, return-to-edit after publication, intelligible progression through
authoring/publication, representative report presentation and responsive states.
The remaining authority-capacity limit is recorded in the acceptance review. Hosted CI remains unavailable under the
owner's local-testing instruction. Production and the open draft PRs stay unmerged.

## Work and acceptance checklist

- [x] Compact host and report workspace use the product's typography, controls and
  tokens; the canvas is visible in the first desktop viewport without scrolling.
- [x] Named report library supports discovery and direct opening of retained
  results; an ordinary reader receives no editing or execution authority.
- [x] An authorized editor can reopen a published report, preserve existing
  draft/review pointers, edit, save, preview and republish through the UI.
- [x] Authoring and publication expose a clear next action, with exact technical
  evidence available on demand and separate effect confirmations preserved.
- [x] Compatible retained data remains useful while arranging and formatting;
  stale/incompatible values are identified and never represented as fresh output.
- [x] A synthetic business report combines KPIs, trend/category charts, a table,
  business filters and narrative context, using actual native retained outputs.
- [x] Desktop, tablet and narrow-screen reading/editing have intentional layouts;
  keyboard operation, focus, contrast, empty/error/partial/stale states are checked.
- [x] Local behavior/race/native tests cover changed seams. Final-source browser
  journeys qualify both transports, including return-to-edit and a separate reader.
- [x] Record realistic report loading/rendering measurements separately from
  source execution. Retained navigation/formatting produce zero source/model work.
- [x] Complete bounded adversarial review, fix reachable defects, update contracts
  and draft PRs, and deliver an independently usable visual walkthrough.

## Invariants

Use one shared application and thin host adapters. No parallel authorization,
duplicate report store, hidden source/model execution, policy broadening or
unbounded asset delivery. An edit proposes a private immutable revision; it never
rewrites a publication. Preserve exact values, required amount disclosure,
retained privacy/expiry, current dependency checks and uncertain-operation custody.

Source comparisons stay outside this repository. All new fixtures are synthetic.
Evidence must identify local component checks, browser fixtures and real-service
acceptance separately. Visual quality is assessed from rendered screens, not from
test counts or the presence of controls.

See [the local acceptance record](../reviews/manual-app-product-quality-2026-10-08.md)
for concrete evidence, review findings and broader-product limits.
