# Naira wiki starter

A VitePress site that builds into a container image. Copy this directory to start a wiki,
or use it as a reference for an image built with another generator.

| Path | Purpose |
|---|---|
| `docs/` | Pages. A file's path is its address. |
| `docs/public/_naira/` | Sign-in page and script, copied into the site as is. |
| `.vitepress/` | Site config and theme (footer, portal bridge). |
| `scripts/write-metadata.mjs` | Writes `dist/naira-wiki.json`. |
| `Dockerfile`, `nginx.conf` | The image. |

## Preview

```sh
npm ci
npm run dev
```

Use the dev server on localhost only. VitePress 1.6.4 depends on Vite 5 and esbuild 0.21, which
have open dev-server advisories (`npm audit`) and no fix in this release line. The build output
and the image do not contain them.

## Build

```sh
NAIRA_WIKI_ID=platform-handbook GITHUB_SHA=$(git rev-parse HEAD) npm run build
```

This writes the site and `naira-wiki.json` to `dist/`. A broken internal link fails the build.
`npm test` runs the unit tests and `npm run typecheck` checks the theme.

## Image

```sh
docker build \
  --build-arg NAIRA_WIKI_ID=platform-handbook \
  --build-arg GITHUB_SHA=$(git rev-parse HEAD) \
  -t platform-handbook .
docker run --rm --read-only --tmpfs /tmp -p 8080:8080 platform-handbook
```

Pin the image by digest wherever you deploy it.

### Image contract

A wiki image, whatever generator built it:

- serves the site at `/`, built with `base: '/'` so it does not depend on its path;
- serves `/naira-wiki.json`;
- serves `/_naira/bootstrap.html` and the files it loads from `/_naira/assets/`;
- listens on port 8080 as a non-root user;
- runs with a read-only root filesystem and a writable `/tmp`.

### naira-wiki.json

Fields without a value are left out.

| Field | Source |
|---|---|
| `contract` | Fixed: `naira.wiki/v1alpha1`. |
| `id` | `NAIRA_WIKI_ID`, else the package `name`. A DNS label of at most 42 characters. |
| `title`, `owner` | `naira.title` and `naira.owner` in `package.json`. |
| `source.repository` | `repository` in `package.json`. |
| `source.commit` | `GITHUB_SHA`. Omitted when unset. |
| `support` | `bugs` in `package.json`. |
| `version` | `version` in `package.json`. |
| `builtAt` | Build time, UTC. |
| `generator` | `vitepress` and its installed version. |

The site footer shows owner, source, version and build time from this file.

### Sign-in files

`/_naira/session` is not served by the image. Whatever fronts the image has to accept a POST
there and set the session cookie.

`bootstrap.html` is the page for a visitor without a session. It waits for the Luigi `init`
message from the portal, posts the access token to `/_naira/session` in the `x-naira-token`
header, then opens the path in `?return=`. Only paths on the same origin are accepted.
`luigi-client.js` is copied from the pinned `@luigi-project/client` when the site is built.

The browser has to keep the session cookie inside the portal's iframe. That works only when the
wiki address is on the same site as the portal, for example `naira.example.com` and
`handbook.wiki.naira.example.com`. After three attempts within 30 seconds without a kept session,
`bootstrap.html` stops and shows a message.

`.vitepress/theme/luigi-bridge.ts` runs only inside a Luigi iframe. It mirrors in-site
navigation into the portal address and posts a refreshed token to `/_naira/session`.

## Upgrade rules

- **Contract.** Additive changes keep `naira.wiki/v1alpha1`. Removing or renaming a path or a
  field needs a new contract string.
- **Page addresses.** An address is the file's path under `docs/`. Do not rename a file without
  keeping a page at the old path. Do not change `base`, `cleanUrls` or `srcDir`.
- **VitePress.** Change the pinned version, run `npm install`, build, and compare the list of
  pages in `dist/` before and after. No address may change.
- **Luigi client.** Keep `@luigi-project/client` on the version of Luigi core in
  `naira-openmfp-portal/frontend/package-lock.json`.
