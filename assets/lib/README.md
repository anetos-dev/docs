# Third-party scripts

The theme would fetch these from a CDN at build time (Mermaid at
`@latest`); they are kept here instead, at fixed versions, so builds
don't depend on the CDN and don't change on their own.

| File | Package | License |
|---|---|---|
| `flexsearch-0.8.143.bundle.min.js` | [flexsearch](https://www.npmjs.com/package/flexsearch) 0.8.143, `dist/flexsearch.bundle.min.js` | Apache-2.0 ([LICENSE-flexsearch](LICENSE-flexsearch)) |
| `mermaid-11.17.2.min.js` | [mermaid](https://www.npmjs.com/package/mermaid) 11.17.2, `dist/mermaid.min.js` | MIT ([LICENSE-mermaid](LICENSE-mermaid)) |

To update one: `npm pack <package>@<version>`, copy the file from the
tarball's `dist/` under its new name, and change the path in hugo.yaml
(`params.search.flexsearch.js`, `params.mermaid.js`).
