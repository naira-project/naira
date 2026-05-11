// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

import React, { useEffect, useState } from "react";
import { Dataset, listDatasets } from "./api";
import DatasetDetail from "./DatasetDetail";
import "./DatasetCatalog.css";

/**
 * DatasetCatalog is the top-level component for the Naira Dataset Catalog
 * micro-frontend. It fetches generic Dataset resources from the Naira Context
 * Engine (Kubernetes API) and renders a searchable, filterable list.
 *
 * This component is completely unaware of OpenMetadata or any other source
 * catalog – it works exclusively with the abstract Dataset CRD.
 */
const DatasetCatalog: React.FC = () => {
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<Dataset | null>(null);

  useEffect(() => {
    listDatasets()
      .then(setDatasets)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  const filtered = datasets.filter((ds) => {
    const q = search.toLowerCase();
    return (
      ds.metadata.name.toLowerCase().includes(q) ||
      (ds.spec.description ?? "").toLowerCase().includes(q) ||
      (ds.spec.owner ?? "").toLowerCase().includes(q) ||
      (ds.spec.tags ?? []).some((t) => t.toLowerCase().includes(q))
    );
  });

  if (selected) {
    return (
      <DatasetDetail
        dataset={selected}
        onBack={() => setSelected(null)}
      />
    );
  }

  return (
    <div className="catalog">
      <header className="catalog-header">
        <h1>Dataset Catalog</h1>
        <p className="catalog-subtitle">
          Discover and explore data assets available in your organisation.
        </p>
        <input
          className="catalog-search"
          type="search"
          placeholder="Search by name, description, owner, or tag…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search datasets"
        />
      </header>

      {loading && <p className="catalog-state">Loading datasets…</p>}
      {error && (
        <p className="catalog-state catalog-error" role="alert">
          Error: {error}
        </p>
      )}

      {!loading && !error && filtered.length === 0 && (
        <p className="catalog-state">No datasets found.</p>
      )}

      {!loading && !error && filtered.length > 0 && (
        <ul className="dataset-list" role="list">
          {filtered.map((ds) => (
            <li key={`${ds.metadata.namespace}/${ds.metadata.name}`} className="dataset-card">
              <button
                className="dataset-card-button"
                onClick={() => setSelected(ds)}
                aria-label={`View details for ${ds.metadata.name}`}
              >
                <div className="dataset-card-header">
                  <span className="dataset-name">{ds.metadata.name}</span>
                  {ds.spec.qualityScore !== undefined && (
                    <span
                      className={`quality-badge quality-${qualityLevel(ds.spec.qualityScore)}`}
                      title="Quality score"
                    >
                      ✦ {ds.spec.qualityScore}
                    </span>
                  )}
                </div>

                {ds.spec.description && (
                  <p className="dataset-description">{ds.spec.description}</p>
                )}

                <div className="dataset-meta">
                  {ds.spec.owner && (
                    <span className="meta-item">👤 {ds.spec.owner}</span>
                  )}
                  {ds.spec.sourceSystem && (
                    <span className="meta-item source-badge">
                      {ds.spec.sourceSystem}
                    </span>
                  )}
                </div>

                {(ds.spec.tags ?? []).length > 0 && (
                  <div className="tag-list" aria-label="Tags">
                    {(ds.spec.tags ?? []).map((tag) => (
                      <span key={tag} className="tag">
                        {tag}
                      </span>
                    ))}
                  </div>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

function qualityLevel(score: number): "high" | "medium" | "low" {
  if (score >= 80) return "high";
  if (score >= 50) return "medium";
  return "low";
}

export default DatasetCatalog;
