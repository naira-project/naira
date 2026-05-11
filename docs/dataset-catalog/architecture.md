# Dataset Catalog – Architecture

<!--
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
SPDX-License-Identifier: Apache-2.0
-->

## Overview

The Naira Dataset Catalog provides AI engineers with a centralised view of available data assets, enabling confident dataset discovery for model fine-tuning, RAG pipelines, and evaluation tasks. The implementation is divided into three strictly separated layers to ensure the Naira core remains tool-agnostic and future-proof.

---

## Architecture Boundaries

```
┌─────────────────────────────────────────────────────────────────────┐
│                        OpenMFP / Luigi UI                           │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │              Dataset Catalog Micro-Frontend                   │  │
│  │         (ui/dataset-catalog – React / TypeScript)            │  │
│  │                                                               │  │
│  │  Queries ONLY: GET /apis/naira.io/v1alpha1/datasets          │  │
│  │  Zero knowledge of OpenMetadata or any source catalog        │  │
│  └───────────────────────────────────────────────────────────────┘  │
└────────────────────────────┬────────────────────────────────────────┘
                             │ Kubernetes API (CRDs)
┌────────────────────────────▼────────────────────────────────────────┐
│                    Naira Context Engine                              │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  Dataset CRD  (naira.io/v1alpha1)                          │    │
│  │  – name, description, owner                                │    │
│  │  – schema (columns + types)                                │    │
│  │  – tags, qualityScore                                      │    │
│  │  – sourceRegistryURL, sourceSystem                         │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  DatasetReconciler  (internal/controller)                   │    │
│  │  – lifecycle management, invariant enforcement              │    │
│  │  – source-agnostic (no catalog-specific code)               │    │
│  └─────────────────────────────────────────────────────────────┘    │
└────────────────────────────▲────────────────────────────────────────┘
                             │ create/update Dataset CRs
┌────────────────────────────┴────────────────────────────────────────┐
│                  OpenMetadata Adapter (Plugin)                       │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  OpenMetadataAdapter CRD  (naira.io/v1alpha1)              │    │
│  │  – url, authSecretRef, syncInterval                        │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  openmetadata.Reconciler  (adapters/openmetadata)           │    │
│  │  – watches OpenMetadataAdapter CRs                          │    │
│  │  – loads token from Kubernetes Secret                       │    │
│  │  – calls OpenMetadata REST API with pagination              │    │
│  │  – maps Table → Dataset via mapper.go                       │    │
│  │  – creates/updates Dataset CRs in target namespace          │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  openmetadata.Client  (adapters/openmetadata/client.go)     │    │
│  │  – GET /api/v1/tables with cursor-based pagination          │    │
│  │  – Bearer token authentication                              │    │
│  └─────────────────────────────────────────────────────────────┘    │
└────────────────────────────▲────────────────────────────────────────┘
                             │ REST API
               ┌─────────────┴──────────────┐
               │   OpenMetadata Instance     │
               └─────────────────────────────┘
```

---

## Component Descriptions

### 1. Dataset CRD (`api/v1alpha1/dataset_types.go`)

The universal, tool-agnostic representation of a data asset. Key fields:

| Field | Purpose |
|---|---|
| `spec.description` | Human-readable dataset description |
| `spec.owner` | Responsible team or person |
| `spec.schema.columns` | Column names, types, descriptions, and column-level tags |
| `spec.tags` | Dataset-level governance labels (e.g., `PII.Email`, `Finance.PCI`) |
| `spec.qualityScore` | Numeric quality score 0–100 from source catalog |
| `spec.sourceRegistryURL` | Deep-link URL back to the asset in the origin catalog |
| `spec.sourceSystem` | Canonical identifier of the source catalog (e.g., `openmetadata`) |

### 2. OpenMetadata Adapter (`adapters/openmetadata/`)

The **only** component in Naira that contains OpenMetadata-specific logic.

- `client.go` – Thin HTTP client for the OpenMetadata REST API. Handles authentication (Bearer token) and transparent cursor-based pagination.
- `mapper.go` – Translates OpenMetadata `Table` entities into Naira `Dataset` resources. This is the canonical translation boundary.
- `controller.go` – Kubernetes reconciler that watches `OpenMetadataAdapter` CRs and drives periodic synchronisation.

### 3. Core Dataset Controller (`internal/controller/dataset_controller.go`)

A source-agnostic reconciler responsible for Dataset lifecycle management. It imposes no knowledge of the origin catalog.

### 4. Dataset Catalog UI (`ui/dataset-catalog/`)

A React/TypeScript micro-frontend designed for embedding in OpenMFP via Luigi. It:
- Queries `Dataset` resources from the Kubernetes aggregated API.
- Provides search/filter, card-based list view, and a detail view with schema table.
- Renders deep-links to the source catalog (`spec.sourceRegistryURL`) while displaying no knowledge of the source tool.

---

## Security Model

- OpenMetadata API credentials are stored in Kubernetes Secrets and never surfaced to end users or logged.
- The adapter's RBAC ClusterRole grants `get` access to Secrets only (not `list` or `watch`), scoped to the adapter's namespace.
- The UI communicates exclusively with the Kubernetes API server using the standard kube-apiserver authentication mechanisms; it never directly contacts OpenMetadata.

---

## Exchangeability

To replace OpenMetadata with a different catalog (e.g., Amundsen, Datahub, Collibra):

1. Write a new adapter package under `adapters/<new-catalog>/`.
2. Create a corresponding configuration CRD.
3. Register the new reconciler in `cmd/dataset-catalog/main.go`.
4. The Dataset CRD, DatasetReconciler, and UI remain **unchanged**.

---

## Synchronisation Flow

```
1. Platform admin applies an OpenMetadataAdapter CR with URL and authSecretRef.
2. openmetadata.Reconciler detects the CR and reads the token from the Secret.
3. Reconciler calls OpenMetadata /api/v1/tables, following pagination.
4. Each Table is passed to MapTableToDataset(), producing a Dataset CR.
5. Reconciler creates or updates Dataset CRs in the target namespace.
6. Reconciler updates the adapter status (lastSyncedAt, datasetsDiscovered).
7. After syncInterval elapses, the reconciler repeats from step 2.
8. The UI fetches /apis/naira.io/v1alpha1/datasets and renders the result.
```

---

## File Layout

```
api/
  v1alpha1/
    groupversion_info.go          – scheme registration
    dataset_types.go              – Dataset CRD Go types
    openmetadataadapter_types.go  – OpenMetadataAdapter CRD Go types
    zz_generated.deepcopy.go      – generated DeepCopy methods

adapters/
  openmetadata/
    client.go                     – OpenMetadata REST API client
    mapper.go                     – Table → Dataset translation
    mapper_test.go                – unit tests for mapper
    controller.go                 – Kubernetes reconciler

cmd/
  dataset-catalog/
    main.go                       – controller manager entry point

config/
  crd/bases/
    naira.io_datasets.yaml
    naira.io_openmetadataadapters.yaml
  rbac/
    role.yaml                     – ServiceAccount + ClusterRole + ClusterRoleBinding
  manager/
    deployment.yaml               – Deployment manifest
  samples/
    v1alpha1_dataset.yaml
    v1alpha1_openmetadataadapter.yaml

internal/
  controller/
    dataset_controller.go         – core Dataset lifecycle controller

ui/
  dataset-catalog/
    src/
      index.tsx
      api.ts                      – Kubernetes API client (Dataset only)
      DatasetCatalog.tsx          – list/search view
      DatasetDetail.tsx           – detail view with schema table
      DatasetCatalog.css
      __tests__/
        DatasetCatalog.test.tsx   – React component tests
    public/
      index.html
    package.json
    tsconfig.json

docs/
  dataset-catalog/
    architecture.md               – this document
```
