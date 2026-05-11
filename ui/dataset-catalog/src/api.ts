// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

/**
 * api.ts – Thin client layer for the Naira Dataset Catalog UI.
 *
 * This module communicates ONLY with the Naira Kubernetes API (via the
 * standard aggregated API server). It knows nothing about OpenMetadata or any
 * other source catalog. All data consumed by the UI originates from generic
 * Dataset custom resources.
 */

export interface DatasetColumn {
  name: string;
  dataType: string;
  description?: string;
  tags?: string[];
}

export interface DatasetSchema {
  columns?: DatasetColumn[];
}

export interface DatasetSpec {
  description?: string;
  owner?: string;
  schema?: DatasetSchema;
  tags?: string[];
  qualityScore?: number;
  sourceRegistryURL?: string;
  sourceSystem?: string;
}

export interface DatasetCondition {
  type: string;
  status: string;
  reason: string;
  message: string;
  lastTransitionTime: string;
}

export interface DatasetStatus {
  conditions?: DatasetCondition[];
  lastSyncedAt?: string;
}

export interface Dataset {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: DatasetSpec;
  status?: DatasetStatus;
}

export interface DatasetList {
  items: Dataset[];
}

/** Base URL for the Kubernetes API. Derived from the current origin in production. */
const API_BASE = process.env.REACT_APP_API_BASE ?? "";

/**
 * Fetches all Dataset resources across all namespaces from the Naira
 * Kubernetes API server. The UI never queries OpenMetadata directly.
 */
export async function listDatasets(namespace?: string): Promise<Dataset[]> {
  const path = namespace
    ? `/apis/naira.io/v1alpha1/namespaces/${encodeURIComponent(namespace)}/datasets`
    : "/apis/naira.io/v1alpha1/datasets";

  const resp = await fetch(`${API_BASE}${path}`, {
    headers: { Accept: "application/json" },
  });

  if (!resp.ok) {
    throw new Error(`Failed to list datasets: ${resp.status} ${resp.statusText}`);
  }

  const data: DatasetList = await resp.json();
  return data.items ?? [];
}

/**
 * Fetches a single Dataset by name and namespace.
 */
export async function getDataset(namespace: string, name: string): Promise<Dataset> {
  const path = `/apis/naira.io/v1alpha1/namespaces/${encodeURIComponent(namespace)}/datasets/${encodeURIComponent(name)}`;

  const resp = await fetch(`${API_BASE}${path}`, {
    headers: { Accept: "application/json" },
  });

  if (!resp.ok) {
    throw new Error(`Failed to get dataset ${name}: ${resp.status} ${resp.statusText}`);
  }

  return resp.json();
}
