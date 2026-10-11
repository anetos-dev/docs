<p align="center">
  <img alt="Anetos" src=".github/assets/anetos-logo-neutral.svg" width="240">
</p>

# docs.anetos.dev

The builder of [docs.anetos.dev](https://docs.anetos.dev), the
documentation of [Anetos](https://github.com/anetos-dev/anetos).

The pages themselves live with the code, in the framework's
[`docs/site`](https://github.com/anetos-dev/anetos/tree/main/docs/site)
folder, so they change in the same pull request as the behavior they
describe. This repository holds the site's theme and build: it pulls
`docs/site` from the framework, once per version, and publishes it. To
fix a page, edit it in anetos-dev/anetos.

## Versions

The site has one version per minor release of the framework, from v0.5
(design D320):

| Address | Pages from |
|---|---|
| `/` | The latest release's newest patch tag (`v0.5.2`) |
| `/main/` | The main branch, labeled unreleased |
| `/v0.5/` | An older release's newest patch tag; the latest's own `/v0.N/` redirects to `/` |

Before the first release (v0.5.0), main is at `/`, and `/main/`
redirects there. `go run ./versions` finds the versions from the
framework's tags (pre-releases left out) and writes the plan to
`.versions/`: `sites.tsv`, one Hugo configuration per version merged
over `hugo.yaml` (its address, the ref its code links point at, the
navbar with the version menu, its banner, `noindex` on all but the
latest, no edit links on older releases), and the `_redirects` lines.
Nothing here changes for a release.

Every version is built with this repository's current `sync`, layouts
and home page (`overlay/`), from its tag's `docs/site`. A change here
must still build the oldest version kept: a version that fails to build
fails the whole build, and Cloudflare Pages then keeps the last good
deployment. Check with `ANETOS_GIT=<a clone of the framework>
./build.sh`. Each version's `_headers` cache rules are repeated under
its path, since Cloudflare Pages reads `_redirects` and `_headers` at
the root only.

## Working on it

[Hugo](https://gohugo.io) (0.158 or later; it's built with 0.167) and the
[Hextra](https://imfing.github.io/hextra/) theme, vendored in `_vendor/`;
Go for the `sync` step.

```sh
ANETOS_DIR=../anetos ./build.sh       # one site, from a local checkout of the framework
./build.sh                            # every version, from anetos-dev/anetos
ANETOS_GIT=../anetos ./build.sh       # every version, from another git repository
ANETOS_REF=v0.5.0 ./build.sh          # one site, from a tag or branch
hugo server                           # after a one-site build: http://localhost:1313
go test ./versions                    # the versions' tests
```

For each site, `build.sh` runs `go run ./sync`, which turns `docs/site` into `content/`
(README.md files become section pages; leading `# Title` headings become
titles; each page learns its path in the framework's repository, so the
relative links between pages, and to examples and code, keep working),
then Hugo. A link to a page that doesn't exist is a build warning.

The sidebar follows the pages' front matter. In each section, a page's
`group:` puts it in a folder of the sidebar named after the group, and
its `weight:` orders it in the group; the groups come in the order of
their pages' weights. Pages keep their URL (`/guides/forms/`), whatever
their group: `sync` sets it, and writes `data/moved.json` so relative
links still find the page. The framework's `make docs-check` (its
`docnav` command) checks that every page has both. Upgrade guides
(`upgrade/v0.3.md`) are ordered newest first, by the version in their
file name.

Hugo reads `{{< … >}}` and `{{% … %}}` in the pages as shortcodes, even
in code blocks; write `{{</* … */>}}` to show one literally.

| Path | What |
|---|---|
| `sync/` | `docs/site` → `content/` (and `data/moved.json`) |
| `versions/` | The versions to build, from the framework's tags (`.versions/`) |
| `layouts/_partials/custom/banner.html` | The banner naming the version, on all but the latest release |
| `layouts/_partials/custom/head-end.html` | Sends the version menu's links to the same page in the other version |
| `layouts/_shortcodes/docs-version.html` | What the home page says about its version |
| `overlay/` | Pages of this site's own, copied over `content/` (the home page) |
| `layouts/_markup/render-link.html` | Resolves the pages' relative links |
| `layouts/_markup/render-image.html` | Resolves the pages' relative images (`./sync` copies docs/site's other files to `files/`) |
| `assets/lib/` | FlexSearch and Mermaid, at fixed versions (see its README) |
| `static/` | Icons and logos, copied from [anetos-dev/website](https://github.com/anetos-dev/website)'s `brand/` |
| `static/_redirects` | `/.well-known/security.txt` to anetos.dev's |

## Deployment

Cloudflare Pages: build command `sh build.sh`, output `public`,
environment variables `HUGO_VERSION=0.167.0` and `GO_VERSION=1.26.8`. It
builds on pushes to this repository; the framework's `Docs` workflow
calls the project's deploy hook when `docs/site` changes on main and
when a release's core tag (`vX.Y.Z`) is pushed.

## License

Apache-2.0, as Anetos.
