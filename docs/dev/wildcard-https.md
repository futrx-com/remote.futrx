# Code Server wildcard HTTPS

Code Server's manifest declares `"web": { "port": 8842, "subdomain": "code" }`.
For project `gamerhead` on `remote.example.com`, it opens at:

```text
https://code--gamerhead.remote.example.com/?folder=%2Fworkspace
```

The manifest label and project slug are dynamic. Remote checks the session,
project membership, and running installation on every application request.
No manual certificate files or export scripts are needed; Caddy persists its
own certificates and renewal state normally.

Preview URLs retain `<slug>--<port>.dev.<host>`. Application web routes require
`web.subdomain` and use `<label>--<project>.<host>`.

## On-demand certificates, no DNS provider

Point `*.<public-host>` at the existing Remote ingress; nothing else is needed.
Named application hosts get certificates the same way previews do: Caddy asks
`/internal/tls-ask` on the first request to a host and issues a certificate
for that one name over HTTP-01/TLS-ALPN. The backend admits only the canonical
`<label>--<project>` host of a running installation, so arbitrary subdomains
cannot spend ACME rate limits. Each project's Code Server host has its own
certificate; there is no shared wildcard certificate and no DNS-01 setup.

Deploy the core gateway and Caddy changes before the Code Server application.
The existing preview configuration needs no changes. Local routing tests cover
this setup with an internal CA. Preserve Caddy's normal storage.
