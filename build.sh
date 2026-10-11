#!/bin/sh
# Builds docs.anetos.dev into public/: one site per version of the docs.
#
#   ./build.sh                          every version, from anetos-dev/anetos
#   ANETOS_GIT=../anetos-git ./build.sh every version, from another git repository
#   ANETOS_DIR=../anetos ./build.sh     one site, from a local checkout's files
#   ANETOS_REF=v0.5.0 ./build.sh        one site, from a branch or tag
#
# Every version: `go run ./versions` reads the framework's tags and plans
# the sites (the latest release at the root, main at /main/, older
# releases at /v0.5/…; main at the root before the first release). Each
# is cloned at its ref, turned into content/ (`go run ./sync`) and built
# by Hugo with its own configuration (.versions/<name>.json) over
# hugo.yaml. The pages' links to code point at the version's ref; their
# edit links at main.
#
# Arguments go to every hugo run (./build.sh --logLevel info). Needs git,
# Go (for ./sync and ./versions) and Hugo (HUGO_VERSION in Cloudflare
# Pages); the theme is vendored in _vendor/.
set -eu
repo="${ANETOS_GIT:-https://github.com/anetos-dev/anetos}"
dir="${ANETOS_DIR:-}"
if [ -n "$dir" ]; then
	dir="$(cd "$dir" && pwd)" # relative to where it's run from
fi
case "$repo" in
*://* | *@*:*) ;; # a URL, or user@host:path
*) repo="file://$(cd "$repo" && pwd)" ;; # file://: a shallow clone
esac
cd "$(dirname "$0")"
rm -rf public

# one builds a site: its pages from $1 (a checkout with docs/site), its
# ref $2, into public/$3, with Hugo's extra configuration $4 (or none);
# the other arguments go to hugo.
one() {
	src="$1" ref="$2" dest="$3" extra="$4"
	shift 4
	go run ./sync -src "$src/docs/site" -out content -files files
	config=hugo.yaml
	if [ -n "$extra" ]; then
		config="hugo.yaml,$extra"
	fi
	HUGO_PARAMS_ANETOSREF="$ref" hugo --gc --minify --config "$config" --destination "public/$dest" "$@"
}

if [ -n "$dir" ] || [ -n "${ANETOS_REF:-}" ]; then
	ref="${ANETOS_REF:-main}"
	if [ -z "$dir" ]; then
		rm -rf .anetos
		git clone --quiet --depth 1 --branch "$ref" "$repo" .anetos
		dir=.anetos
	fi
	one "$dir" "$ref" . "" "$@"
	exit 0
fi

go run ./versions -repo "$repo" -out .versions
# The loop reads the plan from descriptor 3, so the commands in it can't
# consume it from standard input.
while read -r name ref path <&3; do
	rm -rf .anetos
	git clone --quiet --depth 1 --branch "$ref" "$repo" .anetos
	one .anetos "$ref" "$path" ".versions/$name.json" "$@"
	if [ "$path" != . ]; then
		# Cloudflare Pages reads these at the root only.
		rm -f "public/$path/_redirects" "public/$path/_headers"
	fi
done 3< .versions/sites.tsv
cat .versions/_redirects >> public/_redirects
cat .versions/_headers >> public/_headers
