## CI/CD
- Pin all external actions and reusable workflows, including org-internal ones,
  to a full commit SHA with a trailing `# vX.Y.Z` comment so Dependabot/Renovate can bump it.
- Always set `permissions:` explicitly (default contents: `read`) and grant
  `id-token: write` / `packages: write` / `contents: write` only on jobs that publish or release.
- Avoid hardcoding the list of plugins/images in CI workflows — derive it from
  `plugins/cmd/*/` (or equivalent) so adding a plugin doesn't require also remembering to edit
  a workflow file.
- When a script grows past ~10 lines embedded in YAML, move it to its own `.sh` file so
  `shellcheck` can lint it, and keep bash readable (prefer multi-line over dense one-liners —
  obfuscated shell is itself a supply-chain risk, cf. the xz backdoor).


