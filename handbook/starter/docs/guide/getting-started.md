# Getting started

Each Markdown file under `docs/` is a page. Its path is its address, so `docs/guide/getting-started.md`
is served at `/guide/getting-started`.

## Preview

Run `npm run dev` and open the address it prints. The footer with owner and build time appears
only in a built site.

## Add a page

1. Create a file such as `docs/guide/my-page.md`.
2. Add it to the sidebar in `.vitepress/config.mts`.
3. Link to it from other pages with an absolute path, for example [the home page](/).

A link to a page that does not exist fails the build.

## Change an address

Renaming or moving a file changes its address and breaks links that people have shared.
Keep the old path as a page that points to the new one.
