# Test network

Run these commands from the testkit project root, with the publisher and resolver projects checked out next to it (as in `Dev/`): their images build from there.

Sample publishers on fake `.test` domains, for tests and manual experiments.

| Site | What it exercises |
| --- | --- |
| `example-library.test` | Guides; the same content as the golden test vectors |
| `daily-herald.test` | News stories, used for corrections and retractions |
| `devdocs.test` | Technical docs in `<article>`, with tables and code blocks |
| `longform.test` | A long essay with a section over the chunk maximum, a quote, and a list |
| `plain-site.test` | A site that does not take part in sourced.net (marked by a `PLAIN` file) |

Content lives in `../sites/<domain>/public/` as plain HTML, with no sourced files (package `sites` embeds it). Everything signed is generated.

## In tests

Package `testnet` serves these sites inside a Go test: one HTTPS server that routes by host, a throwaway certificate authority that issues a certificate per domain, and a client that trusts it. TLS is fully verified. Each site has fault switches for chaos tests (tampered bundles, wrong or revoked keys, rolled-back manifests, missing links, slow, failing, or dead servers).

```go
n := testnet.Standard(t)             // every sample site, built and served
v := verifier.New(n.Client())
n.Faults("daily-herald.test").TamperBundles()
```

The scenarios are in the resolver project, `verifier/scenario_test.go`.

## With Docker

Caddy serves the prepared sites with certificates from its internal CA. Nothing is exposed except port 8443 on localhost.

```sh
docker/prepare.sh                                   # sign every site into docker/.build (builds ../publisher)
docker compose -f docker/docker-compose.yml up -d   # serve them

# Check a site from inside the network, trusting Caddy's CA:
docker compose -f docker/docker-compose.yml run --rm publisher \
  check -ca /caddy/caddy/pki/authorities/local/root.crt daily-herald.test
```

A resolver runs at `https://resolver.test`, syncing the four publishers. Ask it from the host through the mapped port:

```sh
docker compose -f docker/docker-compose.yml cp sites:/data/caddy/pki/authorities/local/root.crt docker/.build/root.crt
curl --cacert docker/.build/root.crt --resolve resolver.test:8443:127.0.0.1 \
  "https://resolver.test:8443/sourced/v1/fetch?url=https://daily-herald.test/news/2026/09/bridge-reopens.html&query=trucks&max_chunks=1"
```

Save an answer to a file and check it from inside the network, using only public files:

```sh
docker compose -f docker/docker-compose.yml run --rm -v "$PWD/answer.json:/answer.json:ro" publisher \
  check -ca /caddy/caddy/pki/authorities/local/root.crt /answer.json
```

From the host, `curl` can also reach a publisher through the mapped port:

```sh
curl --cacert docker/.build/root.crt --resolve daily-herald.test:8443:127.0.0.1 \
  https://daily-herald.test:8443/.well-known/sourced/manifest.json
```

The sourced tools only accept URLs on port 443, as the spec requires, so run them inside the network as above.

Caddy doesn't send the per-page `Link` header, since that needs per-page server config, so `sourced-publisher check` reports a warning for it. The in-process network does send it.

To change content, edit `sites/`, then run `prepare.sh` again. It starts from scratch each time, with new keys.
