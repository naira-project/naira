// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

package openmetadata

import (
	"strings"

	nairav1alpha1 "github.com/naira-project/naira/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SourceSystem is the canonical identifier written into Dataset.Spec.SourceSystem
	// for all datasets ingested via the OpenMetadata adapter.
	SourceSystem = "openmetadata"

	// ManagedByLabel is the label applied to all Dataset resources created by
	// this adapter so they can be listed and garbage-collected efficiently.
	ManagedByLabel = "naira.io/managed-by"

	// ManagedByValue is the value of the ManagedByLabel for datasets created by
	// the OpenMetadata adapter.
	ManagedByValue = "openmetadata-adapter"

	// AdapterRefLabel stores the name of the OpenMetadataAdapter that owns a
	// Dataset resource, enabling multi-instance support.
	AdapterRefLabel = "naira.io/adapter-ref"
)

// MapTableToDataset translates a single OpenMetadata Table entity into a
// Naira Dataset resource. The returned Dataset is ready to be applied via
// server-side apply. This function is the sole location where OpenMetadata's
// schema is converted to Naira's universal representation.
//
// Parameters:
//   - table:       the OpenMetadata Table entity to translate.
//   - adapterName: the name of the OpenMetadataAdapter resource that triggered
//     the sync (used to populate ownership labels).
//   - namespace:   the Kubernetes namespace in which the Dataset will live.
//   - tableURL:    the deep-link URL pointing back to this table in the
//     OpenMetadata UI.
func MapTableToDataset(table Table, adapterName, namespace, tableURL string) nairav1alpha1.Dataset {
	dataset := nairav1alpha1.Dataset{
		TypeMeta: metav1.TypeMeta{
			APIVersion: nairav1alpha1.GroupVersion.String(),
			Kind:       "Dataset",
		},
		ObjectMeta: metav1.ObjectMeta{
			// Use the fully-qualified OpenMetadata name as the Kubernetes name,
			// sanitised to comply with RFC 1123.
			Name:      sanitizeName(table.FullyQualifiedName),
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
				AdapterRefLabel: adapterName,
			},
			Annotations: map[string]string{
				// Preserve the original FQN so it can be reconstructed later.
				"naira.io/source-fqn": table.FullyQualifiedName,
			},
		},
		Spec: nairav1alpha1.DatasetSpec{
			Description:       table.Description,
			SourceRegistryURL: tableURL,
			SourceSystem:      SourceSystem,
			Tags:              mapTags(table.Tags),
			Schema:            mapSchema(table.Columns),
		},
	}

	if table.Owner != nil {
		dataset.Spec.Owner = table.Owner.Name
	}

	return dataset
}

// mapSchema translates an OpenMetadata column list to a DatasetSchema.
func mapSchema(cols []OMColumn) *nairav1alpha1.DatasetSchema {
	if len(cols) == 0 {
		return nil
	}
	schema := &nairav1alpha1.DatasetSchema{
		Columns: make([]nairav1alpha1.DatasetColumn, 0, len(cols)),
	}
	for _, c := range cols {
		schema.Columns = append(schema.Columns, nairav1alpha1.DatasetColumn{
			Name:        c.Name,
			DataType:    c.DataType,
			Description: c.Description,
			Tags:        mapTags(c.Tags),
		})
	}
	return schema
}

// mapTags extracts the tag FQN strings from an OpenMetadata tag list.
func mapTags(tags []OMTag) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.TagFQN)
	}
	return out
}

// sanitizeName converts an OpenMetadata fully-qualified name (which uses dots
// and other separators) into a valid Kubernetes resource name (RFC 1123).
// Example: "default.bigquery.prod.orders" → "default-bigquery-prod-orders"
func sanitizeName(fqn string) string {
	r := strings.NewReplacer(
		".", "-",
		"_", "-",
		"/", "-",
		" ", "-",
	)
	name := strings.ToLower(r.Replace(fqn))
	// Trim to the maximum Kubernetes name length of 253 characters.
	if len(name) > 253 {
		name = name[:253]
	}
	return name
}
