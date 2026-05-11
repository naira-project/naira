// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import { Dataset } from "./api";
import "./DatasetCatalog.css";

interface Props {
  dataset: Dataset;
  onBack: () => void;
}

/**
 * DatasetDetail renders the full metadata view for a single Dataset resource.
 * It includes the schema table, governance tags, quality score, and a deep-link
 * back to the originating data catalog – all sourced from the generic Dataset
 * CRD, with no awareness of the underlying catalog tool.
 */
const DatasetDetail: React.FC<Props> = ({ dataset, onBack }) => {
  const { spec, metadata } = dataset;
  const columns = spec.schema?.columns ?? [];

  return (
    <div className="detail">
      <button className="back-button" onClick={onBack} aria-label="Back to catalog">
        ← Back to Catalog
      </button>

      <header className="detail-header">
        <h2>{metadata.name}</h2>
        {spec.description && (
          <p className="detail-description">{spec.description}</p>
        )}
        {(spec.tags ?? []).length > 0 && (
          <div className="tag-list" aria-label="Tags">
            {(spec.tags ?? []).map((tag) => (
              <span key={tag} className="tag">
                {tag}
              </span>
            ))}
          </div>
        )}
      </header>

      {/* ─── Metadata ─────────────────────────────────────────── */}
      <section className="detail-section">
        <h3>Metadata</h3>
        <dl className="detail-meta-grid">
          {spec.owner && (
            <>
              <dt className="meta-key">Owner</dt>
              <dd className="meta-value">{spec.owner}</dd>
            </>
          )}
          {spec.sourceSystem && (
            <>
              <dt className="meta-key">Source System</dt>
              <dd className="meta-value" style={{ textTransform: "capitalize" }}>
                {spec.sourceSystem}
              </dd>
            </>
          )}
          {spec.qualityScore !== undefined && (
            <>
              <dt className="meta-key">Quality Score</dt>
              <dd className="meta-value">{spec.qualityScore} / 100</dd>
            </>
          )}
          <dt className="meta-key">Namespace</dt>
          <dd className="meta-value">{metadata.namespace}</dd>
          {metadata.creationTimestamp && (
            <>
              <dt className="meta-key">Created</dt>
              <dd className="meta-value">
                {new Date(metadata.creationTimestamp).toLocaleString()}
              </dd>
            </>
          )}
          {spec.sourceRegistryURL && (
            <>
              <dt className="meta-key">Source Link</dt>
              <dd className="meta-value">
                <a
                  className="external-link"
                  href={spec.sourceRegistryURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={`Open ${metadata.name} in ${spec.sourceSystem ?? "source catalog"}`}
                >
                  Open in {spec.sourceSystem ?? "source catalog"} ↗
                </a>
              </dd>
            </>
          )}
        </dl>
      </section>

      {/* ─── Schema ───────────────────────────────────────────── */}
      <section className="detail-section">
        <h3>Schema {columns.length > 0 ? `(${columns.length} columns)` : ""}</h3>
        {columns.length === 0 ? (
          <p style={{ color: "#777", fontSize: "0.9rem" }}>No schema information available.</p>
        ) : (
          <table className="schema-table" aria-label="Dataset schema">
            <thead>
              <tr>
                <th>Column</th>
                <th>Type</th>
                <th>Description</th>
                <th>Tags</th>
              </tr>
            </thead>
            <tbody>
              {columns.map((col) => (
                <tr key={col.name}>
                  <td>
                    <code>{col.name}</code>
                  </td>
                  <td>
                    <code>{col.dataType}</code>
                  </td>
                  <td>{col.description ?? "—"}</td>
                  <td>
                    {(col.tags ?? []).length > 0 ? (
                      <div className="tag-list">
                        {(col.tags ?? []).map((t) => (
                          <span key={t} className="tag">
                            {t}
                          </span>
                        ))}
                      </div>
                    ) : (
                      "—"
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
};

export default DatasetDetail;
