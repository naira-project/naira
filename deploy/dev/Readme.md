## Deployment Guide

### Prerequisites & Order of Execution

Always run `task core:deploy` **first** before deploying any other stacks. The core module includes the Naira catalog, plugins, UI, and cluster definitions.

If you prefer to deploy a stack without the core module, you must ensure a cluster already exists by running:
`task core:cluster:create`



---

### Troubleshooting

#### Error: Unknown containerd config version

* **Symptom:**
```text
...ERROR: unknown containerd config version: 4 (supported versions: 2 and 3)...
```
* **Cause:** Running an older version of `kind`.
* **Resolution:** Activate [mise](https://mise.jdx.dev/) in your shell to use the correct, supported `kind` version. Refer to the [Prerequisites section in the root README](README.md#prerequisites) for detailed setup instructions.