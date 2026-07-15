# Naira UI (Proof of Concept)

React-based frontend for the Naira. It includes a table listing all nodes and catalog graph.

## Tech Stack

- React 19 + TypeScript
- Tailwind CSS 3
- React Flow (xyflow) for graph visualization
- Vite (via Create React App)

## Development

### Setup

```bash
npm install
```

### Start

```bash
npm start
```

The app runs on [http://localhost:3000](http://localhost:3000) and expects the Naira catalog API at `http://localhost:8090`.

### Build

```bash
npm run build
```

Produces a static build in `build/`.

## Deployment

The `ui-poc` is a standalone React app. To showcase its functionalities, we maintain a local Kind cluster setup. See the root [`Taskfile.yml`](../Taskfile.yml) and [`deploy/dev/`](../deploy/dev/) for platform-level deployment tasks.

## Related

- [`catalog/`](../catalog/) — Backend catalog API service
