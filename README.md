<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/anetos-logo-dark.svg">
    <img alt="Anetos" src=".github/assets/anetos-logo.svg" width="240">
  </picture>
</p>

# docs.anetos.dev

The builder of [docs.anetos.dev](https://docs.anetos.dev), the
documentation of [Anetos](https://github.com/anetos-dev/anetos).

The pages themselves live with the code, in the framework's
[`docs/site`](https://github.com/anetos-dev/anetos/tree/main/docs/site)
folder, so they change in the same pull request as the behaviour they
describe. This repository holds the site's theme and build: it pulls
`docs/site` from the framework's main branch and publishes it. To fix a
page, edit it in anetos-dev/anetos.

## Working on it

[Hugo](https://gohugo.io) (0.158 or later; it's built with 0.167) and the
[Hextra](https://imfing.github.io/hextra/) theme, vendored in `_vendor/`;
Go for the `sync` step.

```sh
ANETOS_DIR=../anetos ./build.sh   # pages from a local checkout of the framework
./build.sh                        # pages from anetos-dev/anetos at main
ANETOS_REF=v0.3.0 ./build.sh      # pages from a tag
hugo server                       # after a build: http://localhost:1313
```

`build.sh` runs `go run ./sync`, which turns `docs/site` into `content/`
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
`docnav` command) checks that every page has both.

Hugo reads `{{< … >}}` and `{{% … %}}` in the pages as shortcodes, even
in code blocks; write `{{</* … */>}}` to show one literally.

| Path | What |
|---|---|
| `sync/` | `docs/site` → `content/` (and `data/moved.json`) |
| `overlay/` | Pages of this site's own, copied over `content/` (the home page) |
| `layouts/_markup/render-link.html` | Resolves the pages' relative links |
| `layouts/_markup/render-image.html` | Resolves the pages' relative images (`./sync` copies docs/site's other files to `files/`) |
| `assets/lib/` | FlexSearch and Mermaid, at fixed versions (see its README) |
| `static/` | Icons and logos, copied from [anetos-dev/website](https://github.com/anetos-dev/website)'s `brand/` |

## Deployment

Cloudflare Pages: build command `sh build.sh`, output `public`,
environment variables `HUGO_VERSION=0.167.0` and `GO_VERSION=1.26.8`. It
builds on pushes to this repository; the framework's CI calls the
project's deploy hook when `docs/site` changes on main.

## License

Apache-2.0, as Anetos.
