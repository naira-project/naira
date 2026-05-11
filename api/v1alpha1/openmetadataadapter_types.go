// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenMetadataAdapterSpec defines the configuration for connecting to an
// OpenMetadata instance and syncing its entities into Naira Dataset resources.
// All OpenMetadata-specific logic is confined to this adapter; the Naira core
// and UI remain fully agnostic to OpenMetadata.
type OpenMetadataAdapterSpec struct {
	// URL is the base URL of the target OpenMetadata instance
	// (e.g., "https://openmetadata.example.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	URL string `json:"url"`

	// AuthSecretRef references a Kubernetes Secret that contains the
	// OpenMetadata API token. The Secret must have a key named "token".
	// Credentials are never surfaced to end users.
	// +kubebuilder:validation:Required
	AuthSecretRef corev1.SecretKeySelector `json:"authSecretRef"`

	// SyncInterval is the period between synchronization runs expressed as a
	// Go duration string (e.g., "5m", "1h"). Defaults to "10m".
	// +optional
	// +kubebuilder:default="10m"
	SyncInterval string `json:"syncInterval,omitempty"`

	// TargetNamespace is the Kubernetes namespace where synced Dataset
	// resources will be created. Defaults to the adapter's own namespace.
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`
}

// OpenMetadataAdapterStatus defines the observed state of an
// OpenMetadataAdapter.
type OpenMetadataAdapterStatus struct {
	// Conditions hold the latest available observations of the adapter's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncedAt records when the most recent successful sync completed.
	// +optional
	LastSyncedAt *metav1.Time `json:"lastSyncedAt,omitempty"`

	// DatasetsDiscovered is the total number of Dataset resources created or
	// updated in the most recent sync run.
	// +optional
	DatasetsDiscovered int32 `json:"datasetsDiscovered,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=omadapter,categories=naira
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Interval",type=string,JSONPath=`.spec.syncInterval`
// +kubebuilder:printcolumn:name="Datasets",type=integer,JSONPath=`.status.datasetsDiscovered`
// +kubebuilder:printcolumn:name="Last Sync",type=date,JSONPath=`.status.lastSyncedAt`

// OpenMetadataAdapter configures and drives the OpenMetadata → Naira Dataset
// synchronization. It is the sole component aware of OpenMetadata specifics;
// no OpenMetadata logic exists in the Naira core or UI layers.
type OpenMetadataAdapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenMetadataAdapterSpec   `json:"spec,omitempty"`
	Status OpenMetadataAdapterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenMetadataAdapterList contains a list of OpenMetadataAdapter resources.
type OpenMetadataAdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenMetadataAdapter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenMetadataAdapter{}, &OpenMetadataAdapterList{})
}
