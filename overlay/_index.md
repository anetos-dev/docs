---
title: Anetos documentation
linkTitle: Overview
description: The documentation of Anetos, a batteries-included web framework for Go.
editURL: https://github.com/anetos-dev/docs/edit/main/overlay/_index.md
---

Anetos is a batteries-included web framework for Go: routing, data,
accounts, queues, mail, storage, search, AI and translations, built on
`net/http`, in one binary.

{{< callout type="info" >}}
Anetos is pre-release. These pages follow the framework's main branch,
where v0.3, the first public version, is being finished; APIs may change
until 1.0.
{{< /callout >}}

{{< cards >}}
  {{< card link="getting-started/" title="Getting started" icon="play" subtitle="Create a project, run it with live reload, and add a model, a migration and a page." >}}
  {{< card link="guides/" title="Guides" icon="book-open" subtitle="How to do one thing, step by step: forms, accounts, queues, mail, search, AI…" >}}
  {{< card link="concepts/" title="Concepts" icon="light-bulb" subtitle="How Anetos works, and why: the request lifecycle, the data layer, the supervisor." >}}
  {{< card link="reference/" title="Reference" icon="collection" subtitle="Settings, commands, struct tags, validation rules and formats." >}}
{{< /cards >}}

## Where to start

- New to Anetos? [Getting started](getting-started/) builds a small app in
  about fifteen minutes.
- Coming from Laravel or Rails? The [guides](guides/) map what you know:
  routes, models, migrations, queues and mail work much as you expect,
  with types.
- Upgrading? The [upgrade guides](upgrade/) list what changes between
  releases, and how to move your code.

Found a mistake? Every page has an "Edit this page" link: the pages live
with the code, in the framework's
[`docs/site`](https://github.com/anetos-dev/anetos/tree/main/docs/site)
folder.
