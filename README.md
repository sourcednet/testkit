# testkit

What the other projects test against: a fake HTTPS network in one process, sample sites, and chaos modes, plus a Docker test network with real servers. Used by tests and by `sourced-lab demo`; nothing here is deployed.

```
make            # lint and build
make testnet    # sign the sample sites into docker/.build (needs ../publisher)
make testnet-up # serve them with Caddy, plus a resolver, in Docker (see docker/README.md)
```

## Packages

- `fakenet`: one HTTPS server routing by host, with a throwaway CA issuing a certificate per fake domain on demand, a client that trusts it, and fault switches (tampered bundles, wrong or revoked keys, rolled-back manifests, slow, failing, or dead servers).
- `testnet`: `fakenet` for tests: `testnet.Standard(t)` serves every sample site, signed.
- `testsite`: builds realistic publisher projects for tests.
- `sites`: the sample sites, embedded, so any project's tests find them: `daily-herald.test` (news), `example-library.test` (guides), `devdocs.test` (docs with tables and code), `longform.test` (a long essay), and `plain-site.test` (doesn't take part).
