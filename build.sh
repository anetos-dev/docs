#!/bin/sh
# Builds docs.anetos.dev into public/.
#
#   ./build.sh                       pages from anetos-dev/anetos at main
#   ANETOS_REF=v0.3.0 ./build.sh     pages from a tag
#   ANETOS_DIR=../anetos ./build.sh  pages from a local checkout
#
# The pages' links to code point at ANETOS_REF; their edit links at main.
# Needs Go (for ./sync) and Hugo (HUGO_VERSION in Cloudflare Pages); the
# theme is vendored in _vendor/.
set -eu
ref="${ANETOS_REF:-main}"
dir="${ANETOS_DIR:-}"
if [ -n "$dir" ]; then
	dir="$(cd "$dir" && pwd)" # relative to where it's run from
fi
cd "$(dirname "$0")"
if [ -z "$dir" ]; then
	rm -rf .anetos
	git clone --quiet --depth 1 --branch "$ref" https://github.com/anetos-dev/anetos .anetos
	dir=.anetos
fi
go run ./sync -src "$dir/docs/site" -out content -files files
HUGO_PARAMS_ANETOSREF="$ref" hugo --gc --minify "$@"
