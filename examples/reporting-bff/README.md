# Client-owned reporting iframe example

`internal/rendering/bffexample` is a concrete server-side forwarding example. The
client BFF authenticates its own browser session, obtains a current Pengui bearer
through its existing server-side provider, and posts a closed render request to
Chartworks. It never forwards a browser bearer, stores a token in HTML or creates
an embed grant, bootstrap code, cookie or Chartworks session. The returned static
HTML/SVG has no client chart JavaScript and is served with `private, no-store` and
an explicit `frame-ancestors` policy.

The BFF's cache, if one is added by an adopter, must be private and keyed by the
complete authorized artifact/context coordinate. This example deliberately does
not include a cache or local credential issuer.
