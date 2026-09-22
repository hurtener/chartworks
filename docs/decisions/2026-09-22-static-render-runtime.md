# Static rendering runtime decision

### D-083 — Durable renditions are immutable retained-artifact derivatives

Status: implemented for review, 2026-09-22. Owns Phase 32 and extends D-079.

Chartworks renders only sealed retained output contracts. HTML tables, KPI/text
content and report/dashboard geometry are generated from exact retained values;
chart SVG crosses a fixed-argument supervised process with an empty credential
environment, bounded input/output/time/memory/concurrency and one-request crash
isolation. No URL, script, source, model or bearer is part of the worker protocol.

Renditions are immutable PostgreSQL records keyed by artifact projection,
renderer/theme version, format and viewport. Reads recheck current artifact reach;
expiry deletes bytes and regeneration under a new renderer/theme version creates
a distinct identity. Creation and expiry append content-free audit events. JSON/CSV/
HTML/SVG are the complete export matrix. PDF and PNG remain absent. The client-owned
BFF forwards a fresh Pengui bearer server-side and Chartworks adds no embed token/
session/issuer surface.
