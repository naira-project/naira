# Naira How-To

Task guides for people who use and run Naira: run it locally, work with the catalog, install and
configure it, and write a plugin. It is a wiki built from [`wiki/starter`](../starter/README.md).

| Path | Purpose |
|---|---|
| `docs/` | The pages |
| `.vitepress/config.mts` | The navigation and the sidebar |
| `package.json` | The wiki's id (`howto`), title and owner |

Everything else is the starter's, unchanged. Preview, build, the image contract and the upgrade
rules are in the [starter README](../starter/README.md).

```sh
npm ci
npm run dev
docker build -t howto .
```

`values-dev.yaml` installs it for local development, and Tilt builds it as
`local/naira-wiki-howto`. A claim on a page has to hold for the code on `main`: the home page names
the commit the guides were checked against.
