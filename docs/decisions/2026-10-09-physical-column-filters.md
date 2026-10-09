# Physical column filters

### D-108 — Typed filters bound to registered physical columns

Accepted scope, 2026-10-09. Extends D-106/D-107 and the existing parameter binder;
reviewed dimension references and old serialized definitions remain unchanged.

An author may filter a registered physical column without creating a reviewed
dimension. The request selects its catalog ID. The server compiles a typed
parameter with its actual column name, registered source/context/dataset pin,
source revision and schema digest. Physical origin and reviewed meaning are
exclusive references; neither a label nor a user-selected type supplies proof.

`column_value`, `column_set` and `column_range` retain exact values. Sets use the
existing bounded positive-IN proof and null padding. Ranges have inclusive lower
and exclusive upper bounds. Every protected slot is used once in a root WHERE
conjunction on the exact bound column. Native authoring/import and saved report
execution use the same declaration, SQL-placement and source-identity checks.
Report filter compatibility includes the complete physical reference.

Dates and civil timestamps require the explicit Gregorian calendar and no zone.
Instant ranges resolve civil timestamps in an explicit IANA zone, rejecting DST
gaps/folds. Report/session timezone does not alter this choice. Text, UUID,
boolean, integer and decimal choices are governed by actual physical types;
numeric-looking names have no special meaning. No implicit joins, expressions,
null-selection semantics or arbitrary SQL are introduced.

No storage migration is required: the optional fields live in already versioned,
immutable JSON definitions and participate in their existing hashes. Empty new
fields remain omitted for old definitions. Existing custody, source authority,
validation and publication fences remain mandatory.

Private predecessor review supports separating typed filter definitions, exact
field bindings, saved defaults and temporary reader selections. Option discovery
is independently explicit source work. No source code or private schemas are
copied. The native implementation does not establish physical option search or
Builder controls; those remain open in the flexible-authoring goal.
