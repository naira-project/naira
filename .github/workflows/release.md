# Release Process

Releases are automated via [Release Please](https://github.com/googleapis/release-please). Version bumps and changelogs are derived from [Conventional Commits](https://www.conventionalcommits.org/).

---

## Pull Request Title Format

```
<type>: <description>

# Examples
feat: add user authentication
fix: correct null pointer in catalog loader
chore: update dependencies
```

| Type | Version bump |
|------|-------------|
| `feat` | minor (`0.x.0`) |
| `fix` | patch (`0.0.x`) |
| `chore`, `docs`, etc. | none |
| `feat!` or `BREAKING CHANGE` | major (`x.0.0`) |

---

## Lifecycle

```
feat/fix pull requests → main (squash merged)
        ↓
Release Please opens RC PR  (e.g. v0.12.0-rc.1)
        ↓
Merge RC PR → silent tag pushed, Docker image built (no public GitHub Release)
        ↓
Run "Promote RC to Stable" workflow (manual)
        ↓
Release Please opens Stable PR  (e.g. v0.12.0)
        ↓
Merge Stable PR → GitHub Release published, Docker image built
        ↓
"Start Next RC Cycle" triggers automatically → resets config for next RC
```

---

## For Developers

**Day-to-day:** Use a Conventional Commit title for each PR and squash merge it. Individual commits within the PR do not need to follow the Conventional Commits specification. Release Please handles the rest.

**To release a new stable version:**

1. Go to **Actions → Promote RC to Stable → Run workflow**
2. Wait for Release Please to open a Stable PR (titled `chore(main): release x.y.z`)
3. Review the squashed `CHANGELOG.md` — **edit it directly on the branch if needed** (push a commit to the release branch before merging)
4. Merge the PR

**To edit the changelog after a release is published:**
```bash
gh release edit v0.12.0 --notes "Your curated release notes here"
```

**Nothing else is needed.** Do not manually edit `release-please-config.json` or `.release-please-manifest.json`.

---

## GitHub Release body

The Release Pipeline automatically populates the GitHub Release body with the squashed changelog entry from `CHANGELOG.md` after a stable release is published. This means all features and fixes from the RC cycle appear in the release notes, not just the promotion commit.

## Artifacts

Docker images are published to the GitHub Container Registry (`ghcr.io`) on every RC tag and every stable release. Images are tagged with the full version (e.g. `v0.12.0-rc.1`, `v0.12.0`).

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Release Please proposes wrong version | Run **Promote RC to Stable** workflow — it heals the manifest and pins `release-as` automatically |
| RC PR not appearing after merging a feature | Check that the squash-merged PR title uses a valid conventional commit type (`feat:` or `fix:`) |
| Stable PR shows 0 user-facing commits | The promotion commit uses `fix:` to wake up Release Please — this is expected |
