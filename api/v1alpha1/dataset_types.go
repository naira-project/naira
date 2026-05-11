// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatasetColumn describes a single column/field in a dataset schema.
type DatasetColumn struct {
	// Name is the column name.
	Name string `json:"name"`
	// DataType is the data type of the column (e.g., STRING, INTEGER, BOOLEAN).
	DataType string `json:"dataType"`
	// Description is an optional human-readable description of the column.
	// +optional
	Description string `json:"description,omitempty"`
	// Tags holds arbitrary labels applied to this column (e.g., PII, SENSITIVE).
	// +optional
	Tags []string `json:"tags,omitempty"`
}

// DatasetSchema describes the structural schema of a dataset.
type DatasetSchema struct {
	// Columns holds the list of columns/fields in the dataset.
	// +optional
	Columns []DatasetColumn `json:"columns,omitempty"`
}

// DatasetSpec defines the desired state of a Dataset.
type DatasetSpec struct {
	// Description is a human-readable explanation of the dataset's purpose.
	// +optional
	Description string `json:"description,omitempty"`

	// Owner is the team or person responsible for this dataset.
	// +optional
	Owner string `json:"owner,omitempty"`

	// Schema describes the structural schema of the dataset (columns and types).
	// +optional
	Schema *DatasetSchema `json:"schema,omitempty"`

	// Tags holds arbitrary classification labels applied to the dataset
	// (e.g., "pii", "gdpr", "finance").
	// +optional
	Tags []string `json:"tags,omitempty"`

	// QualityScore is an optional numeric quality score between 0 and 100,
	// sourced from the underlying data catalog's quality checks.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	QualityScore *int32 `json:"qualityScore,omitempty"`

	// SourceRegistryURL is the deep-link URL to the exact asset page in the
	// originating data catalog (e.g., an OpenMetadata table view URL).
	// +optional
	SourceRegistryURL string `json:"sourceRegistryURL,omitempty"`

	// SourceSystem identifies the external data catalog this dataset was
	// ingested from (e.g., "openmetadata", "amundsen", "datahub").
	// +optional
	SourceSystem string `json:"sourceSystem,omitempty"`
}

// DatasetStatus defines the observed state of a Dataset.
type DatasetStatus struct {
	// Conditions hold the latest available observations of the Dataset's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncedAt is the timestamp of the most recent synchronization from
	// the source data catalog.
	// +optional
	LastSyncedAt *metav1.Time `json:"lastSyncedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ds,categories=naira
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=`.spec.owner`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.sourceSystem`
// +kubebuilder:printcolumn:name="Quality",type=integer,JSONPath=`.spec.qualityScore`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Dataset is the generic, tool-agnostic representation of a data asset within
// the Naira Context Engine. Adapter controllers (e.g., the OpenMetadata adapter)
// translate external catalog entries into Dataset resources; the Naira UI then
// queries only Dataset resources, remaining completely unaware of the source tool.
type Dataset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatasetSpec   `json:"spec,omitempty"`
	Status DatasetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatasetList contains a list of Dataset resources.
type DatasetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Dataset `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Dataset{}, &DatasetList{})
}
