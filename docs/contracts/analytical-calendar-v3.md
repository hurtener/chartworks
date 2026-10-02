# Analytical calendar grain v3

New authoring selects analytical record version 3. Versions 0, 1 and 2 remain
unchanged and rebuild their original policies on execution and replay.

This adds a scoped PostgreSQL single-base calendar partition proof, not general
natural-language correctness, query-wide filters, ordering, joins or approval.
The complete terminal by/per/por clause may name day/month/quarter/year of an
already selected reviewed temporal dimension, or día/mes/trimestre/año de it.
Exact direct dimension labels take precedence. Generic “by month” does not guess
a date column. The existing quoted/private/negative-clause exclusions remain.

The selected dimension must review Gregorian calendar and the requested grain.
Exact native date, timestamp without time zone, and timestamp with time zone
mean different things. A date needs an explicit unzoned timestamp cast before
date_trunc; a civil timestamp needs no zone. An instant requires the exact
reviewed IANA timezone as the third date_trunc argument. Session-zone coercions,
EXTRACT(month), formatted labels, arithmetic source fields and unknown native
types cannot acquire partition equivalence. Civil midnight buckets may be cast
to date; zoned bucket-to-date casts are not proven. Week/hour/fiscal grains,
filtered dimensions and multi-relation shapes remain unsupported.

Native validation still runs first and is the sole issuer of executable plans.
The analytical check compares the complete GROUP BY and projected partition
sets while retaining the aggregate/population/zero-division checks. Wrong grain
uses the existing bounded validation correction; private scalar values remain
server-owned. No model calls are added to frozen report refresh.

Migration 055 accepts v3 without rewriting prior rows or removing the immutable
proof trigger. The contract digest pins grain/calendar/zone/field metadata;
receipt grouping IDs remain reviewed dimension identities. Unknown grouping
remains unmeasured, not a grand-total assertion.

Qualification: synthetic/native unit and real PostgreSQL/recorded-provider
acceptance are required. Live interpretation accuracy, warehouse parity and
query-wide business correctness are not implied by these scoped tests.
