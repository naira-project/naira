# Add a wiki

A wiki is a static site that a team writes in Git and installs in Naira. It opens in the portal's
Docs group, like the one you are reading. Naira does not edit or index wikis.

::: warning Not in a release yet
Wikis are new and are not part of a Naira release at the time of writing. The steps below describe
the version under development. Check the release notes before you rely on them.
:::

## Steps

1. **Start from the starter.** Copy `wiki/starter` from the
   [naira repository](https://github.com/naira-project/naira) into a repository of your own. It is a
   VitePress site that builds into a container image.
2. **Write the pages.** Each Markdown file under `docs/` is a page, and its path is its address.
   List the pages in the sidebar in `.vitepress/config.mts`. In `package.json`, set `naira.title`,
   `naira.owner`, `repository` and `bugs`. The site footer shows them.
3. **Preview it.** Run `npm ci` and `npm run dev`. A broken link between pages fails the build.
4. **Build the image.** Pass the wiki's id and the commit:

   ```sh
   docker build \
     --build-arg NAIRA_WIKI_ID=platform-handbook \
     --build-arg GITHUB_SHA=$(git rev-parse HEAD) \
     -t platform-handbook .
   ```

   The id is a DNS label of at most 42 characters. It becomes the wiki's address and its route in
   the portal.
5. **Publish and sign.** Push the image to a registry your Naira may pull from and sign it. Note its
   digest.
6. **Install it.** Whoever runs your Naira adds the wiki to `wikis` in the chart values, with the
   image by digest:

   ```yaml
   wikis:
     - id: platform-handbook
       contract: naira.wiki/v1alpha1
       label: Platform Handbook
       image: ghcr.io/acme/platform-handbook@sha256:<digest>
   ```

To publish a change, build and push a new image and replace the digest. The address stays the same.

## What the person who runs Naira needs

The wiki gets its own address, so Naira needs wildcard DNS and a wildcard certificate for it. The
address has to be on the same site as the portal, and wiki code runs with the viewer's access token,
so only wikis you trust belong there. The operator guide `docs/wikis.md` in the repository covers
all of this.

## Good to know

- A link to a page opens it in the portal. A link with a `#heading` opens the page at the top for
  now, because the portal drops the heading.
- Every signed-in user can open every wiki. Restricting a wiki to a group is not available.
- This wiki's source is `wiki/howto` in the repository. Another example is `wiki/example`.
