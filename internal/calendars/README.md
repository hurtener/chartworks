# Pinned timezone rules

`zoneinfo.zip` is the exact public timezone archive shipped with Go 1.26.4.
SHA-256: `8f55634d05f8bca1f7bc7c69c5933428c69357e0bdf565e5ba224e3f88ff12e8`. The adjacent Go license is retained. This identifier
describes the actual archive; it is not an invented IANA release label.

Jobs and reporting share `Location`; host TZ/ZONEINFO files are not consulted.
The archive has no customer data, network loaders or credentials. Updating it
is a reviewed data/config migration: update the checksum, test DST/leap/first
occurrences, and record the new version. Already accepted reporting occurrences
keep their due instant, half-open window and resolved parameters. Queue replicas
refuse a different registered timezone archive.
