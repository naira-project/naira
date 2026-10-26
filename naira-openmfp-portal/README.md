# naira-openmfp-portal

OpenMFP Portal

## Prerequisites

- Node.js >= 24.0.0
- npm >= 11.0.0

## Installation

Install dependencies for the root, frontend, and backend:

```bash
npm install
cd frontend && npm install && cd ..
cd backend && npm install && cd ..
```

## Development

Start both frontend and backend in development mode:

```bash
npm start
```

- With `tilt up` running in the repo root, frontend will be available at `http://localhost:3000`.
- Click on the button at the upper right part of the OpenMFP portal and go to `Settings`. There, toggle **Is Development Mode active?** and click on `Save`.

## Build

Build both frontend and backend:

```bash
npm run build
```

## Project Structure

- `backend/` - NestJS backend application
- `frontend/` - Angular frontend application
- `Dockerfile` - Multi-stage build for the production container image

## Available Scripts

- `npm start` - Start both frontend and backend in development mode
- `npm run build` - Build both applications
- `npm run start:ui` - Start only frontend
- `npm run start:server` - Start only backend
- `npm run build:ui` - Build only frontend
- `npm run build:server` - Build only backend

## kind Cluster Deployment

The portal is deployed as a container in the local kind cluster as part of the platform.
Use the Tiltfile in the repo root:

```bash
# Deploy the full platform, port-forwarding the portal to localhost:3000
tilt up

# Deploy only the portal and what it depends on
tilt up portal

# Remove the platform from the cluster
tilt down
```
