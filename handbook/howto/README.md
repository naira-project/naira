# Naira How-To

Task guides for people who use Naira: find your way around the portal, work with the catalog,
maintain the tech radar, and write a plugin. Installing and running Naira is not covered. It is a handbook built from
[`handbook/starter`](../starter/README.md).

| Path | Purpose |
|---|---|
| `docs/` | The pages |
| `.vitepress/config.mts` | The navigation and the sidebar |
| `package.json` | The handbook's id (`howto`), title and owner |

Everything else is the starter's, unchanged. Preview, build, the image contract and the upgrade
rules are in the [starter README](../starter/README.md).

```sh
npm ci
npm run dev
docker build -t howto .
```

`values-dev.yaml` installs it for local development, and Tilt builds it as
`local/naira-handbook-howto`. A claim on a page has to hold for the code on `main`: the home page names
the commit the guides were checked against.
