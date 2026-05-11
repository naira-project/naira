// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

package openmetadata_test

import (
	"testing"

	"github.com/naira-project/naira/adapters/openmetadata"
)

func TestMapTableToDataset_BasicFields(t *testing.T) {
	table := openmetadata.Table{
		ID:                 "abc-123",
		Name:               "orders",
		FullyQualifiedName: "default.bigquery.prod.orders",
		Description:        "Production orders table",
		Owner: &openmetadata.OMOwner{
			Name: "data-team",
			Type: "team",
		},
	}

	ds := openmetadata.MapTableToDataset(table, "my-adapter", "naira-system", "https://om.example.com/table/default.bigquery.prod.orders")

	if ds.Spec.Description != "Production orders table" {
		t.Errorf("expected description %q, got %q", "Production orders table", ds.Spec.Description)
	}
	if ds.Spec.Owner != "data-team" {
		t.Errorf("expected owner %q, got %q", "data-team", ds.Spec.Owner)
	}
	if ds.Spec.SourceSystem != "openmetadata" {
		t.Errorf("expected sourceSystem %q, got %q", "openmetadata", ds.Spec.SourceSystem)
	}
	if ds.Spec.SourceRegistryURL != "https://om.example.com/table/default.bigquery.prod.orders" {
		t.Errorf("unexpected sourceRegistryURL: %q", ds.Spec.SourceRegistryURL)
	}
	if ds.Namespace != "naira-system" {
		t.Errorf("expected namespace %q, got %q", "naira-system", ds.Namespace)
	}
}

func TestMapTableToDataset_NameSanitization(t *testing.T) {
	cases := []struct {
		fqn      string
		wantName string
	}{
		{"default.bigquery.prod.orders", "default-bigquery-prod-orders"},
		{"My_Table.Schema", "my-table-schema"},
		{"simple", "simple"},
	}

	for _, tc := range cases {
		t.Run(tc.fqn, func(t *testing.T) {
			table := openmetadata.Table{
				FullyQualifiedName: tc.fqn,
			}
			ds := openmetadata.MapTableToDataset(table, "adapter", "default", "")
			if ds.Name != tc.wantName {
				t.Errorf("fqn=%q: expected name %q, got %q", tc.fqn, tc.wantName, ds.Name)
			}
		})
	}
}

func TestMapTableToDataset_SchemaMapping(t *testing.T) {
	table := openmetadata.Table{
		FullyQualifiedName: "svc.db.customers",
		Columns: []openmetadata.OMColumn{
			{Name: "id", DataType: "BIGINT", Description: "Primary key"},
			{Name: "email", DataType: "VARCHAR", Tags: []openmetadata.OMTag{{TagFQN: "PII.Email"}}},
		},
	}

	ds := openmetadata.MapTableToDataset(table, "adapter", "default", "")

	if ds.Spec.Schema == nil {
		t.Fatal("expected non-nil schema")
	}
	if len(ds.Spec.Schema.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(ds.Spec.Schema.Columns))
	}

	col0 := ds.Spec.Schema.Columns[0]
	if col0.Name != "id" || col0.DataType != "BIGINT" || col0.Description != "Primary key" {
		t.Errorf("unexpected column[0]: %+v", col0)
	}

	col1 := ds.Spec.Schema.Columns[1]
	if len(col1.Tags) != 1 || col1.Tags[0] != "PII.Email" {
		t.Errorf("unexpected column[1] tags: %+v", col1.Tags)
	}
}

func TestMapTableToDataset_TagMapping(t *testing.T) {
	table := openmetadata.Table{
		FullyQualifiedName: "svc.db.payments",
		Tags: []openmetadata.OMTag{
			{TagFQN: "Finance.PCI"},
			{TagFQN: "PII.Sensitive"},
		},
	}

	ds := openmetadata.MapTableToDataset(table, "adapter", "default", "")

	if len(ds.Spec.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(ds.Spec.Tags))
	}
	if ds.Spec.Tags[0] != "Finance.PCI" || ds.Spec.Tags[1] != "PII.Sensitive" {
		t.Errorf("unexpected tags: %+v", ds.Spec.Tags)
	}
}

func TestMapTableToDataset_NoOwner(t *testing.T) {
	table := openmetadata.Table{
		FullyQualifiedName: "svc.db.logs",
		Owner:              nil,
	}

	ds := openmetadata.MapTableToDataset(table, "adapter", "default", "")
	if ds.Spec.Owner != "" {
		t.Errorf("expected empty owner, got %q", ds.Spec.Owner)
	}
}

func TestMapTableToDataset_ManagedByLabels(t *testing.T) {
	table := openmetadata.Table{FullyQualifiedName: "svc.db.events"}
	ds := openmetadata.MapTableToDataset(table, "my-adapter", "default", "")

	if ds.Labels[openmetadata.ManagedByLabel] != openmetadata.ManagedByValue {
		t.Errorf("expected label %s=%s", openmetadata.ManagedByLabel, openmetadata.ManagedByValue)
	}
	if ds.Labels[openmetadata.AdapterRefLabel] != "my-adapter" {
		t.Errorf("expected label %s=my-adapter", openmetadata.AdapterRefLabel)
	}
}

func TestMapTableToDataset_LongFQNTruncated(t *testing.T) {
	// FQN of 300 characters should be truncated to 253.
	fqn := make([]byte, 300)
	for i := range fqn {
		fqn[i] = 'a'
	}
	table := openmetadata.Table{FullyQualifiedName: string(fqn)}
	ds := openmetadata.MapTableToDataset(table, "adapter", "default", "")
	if len(ds.Name) > 253 {
		t.Errorf("name length %d exceeds 253", len(ds.Name))
	}
}
