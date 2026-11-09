# Example Platform Handbook

An example handbook: a made-up team handbook that shows what a team can keep in a Naira handbook. It is
built from [`handbook/starter`](../starter/README.md), and its pages show the patterns the starter can
render. Nothing on its pages is real.

| Path | Purpose |
|---|---|
| `docs/` | The pages |
| `.vitepress/config.mts` | The navigation and the sidebar |
| `package.json` | The handbook's id (`example`), title and owner |

Everything else is the starter's, unchanged. Preview, build, the image contract and the upgrade
rules are in the [starter README](../starter/README.md).

```sh
npm ci
npm run dev
docker build -t example .
```

`values-dev.yaml` installs it for local development, and Tilt builds it as
`local/naira-handbook-example`.
