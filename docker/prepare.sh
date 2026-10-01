#!/usr/bin/env sh
# Builds every sample site in sites/ into docker/.build/<domain>,
# ready for Caddy to serve: a publisher project per site (keys, sourced.json)
# with its signed web root in public/. Plain sites are copied as they are.
#
# Usage: docker/prepare.sh            (run from the testkit project root)
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)
out="$here/.build"
now="2026-09-28T10:00:00Z"

echo "Building sourced-publisher…"
(cd "$root/../publisher" && go build -o "$out/bin/sourced-publisher" ./cmd/sourced-publisher)
sourced="$out/bin/sourced-publisher"

for site in "$root"/sites/*/; do
	domain=$(basename "$site")
	dest="$out/$domain"
	rm -rf "$dest"
	mkdir -p "$dest"
	if [ -f "$site/PLAIN" ]; then
		cp -R "$site/public" "$dest/public"
		echo "plain      $domain"
		continue
	fi
	"$sourced" init -publisher "$domain" -now "$now" "$dest" >/dev/null
	cp -R "$site/public/." "$dest/public/"
	"$sourced" build -now "$now" "$dest" | tail -n 1 | sed "s/^/published  $domain: /"
	"$sourced" check "$dest" >/dev/null
done

echo "Done. Sites are in $out; start them with: docker compose -f docker/docker-compose.yml up -d"
