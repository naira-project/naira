// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

package openmetadata

import (
	"context"
	"fmt"
	"time"

	nairav1alpha1 "github.com/naira-project/naira/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	conditionTypeSynced = "Synced"
	conditionReasonOK   = "SyncSucceeded"
	conditionReasonFail = "SyncFailed"

	defaultSyncInterval = 10 * time.Minute
)

// Reconciler watches OpenMetadataAdapter resources and drives the periodic
// synchronisation of OpenMetadata tables into Naira Dataset resources.
// It is the only controller in the system that imports or references any
// OpenMetadata-specific types.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager registers the Reconciler with the controller-runtime manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nairav1alpha1.OpenMetadataAdapter{}).
		Complete(r)
}

// Reconcile is called by controller-runtime whenever an OpenMetadataAdapter
// resource is created, updated, or deleted. It loads the OpenMetadata
// credentials from the referenced Secret, invokes the sync logic, and updates
// the adapter's status with the result.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	adapter := &nairav1alpha1.OpenMetadataAdapter{}
	if err := r.Get(ctx, req.NamespacedName, adapter); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve sync interval (default to 10 minutes if unset or invalid).
	syncInterval := defaultSyncInterval
	if adapter.Spec.SyncInterval != "" {
		if d, err := time.ParseDuration(adapter.Spec.SyncInterval); err == nil {
			syncInterval = d
		}
	}

	// Determine the target namespace for Dataset resources.
	targetNS := adapter.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = adapter.Namespace
	}

	// Load the API token from the referenced Secret.
	token, err := r.resolveToken(ctx, adapter)
	if err != nil {
		logger.Error(err, "failed to resolve OpenMetadata token")
		r.setCondition(ctx, adapter, metav1.ConditionFalse, conditionReasonFail, err.Error())
		return ctrl.Result{RequeueAfter: syncInterval}, nil
	}

	// Run the sync.
	omClient := NewClient(adapter.Spec.URL, token)
	discovered, err := r.sync(ctx, omClient, adapter, targetNS)
	if err != nil {
		logger.Error(err, "sync failed")
		r.setCondition(ctx, adapter, metav1.ConditionFalse, conditionReasonFail, err.Error())
		return ctrl.Result{RequeueAfter: syncInterval}, nil
	}

	logger.Info("sync completed", "datasetsDiscovered", discovered)
	now := metav1.Now()
	adapter.Status.LastSyncedAt = &now
	adapter.Status.DatasetsDiscovered = int32(discovered)
	r.setCondition(ctx, adapter, metav1.ConditionTrue, conditionReasonOK, "Sync completed successfully")

	return ctrl.Result{RequeueAfter: syncInterval}, nil
}

// sync fetches all tables from OpenMetadata and upserts corresponding Dataset
// resources in Kubernetes. It returns the number of datasets processed.
func (r *Reconciler) sync(
	ctx context.Context,
	omClient *Client,
	adapter *nairav1alpha1.OpenMetadataAdapter,
	targetNS string,
) (int, error) {
	tables, err := omClient.ListTables(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing tables from OpenMetadata: %w", err)
	}

	for _, table := range tables {
		tableURL := omClient.TableURL(table.FullyQualifiedName)
		desired := MapTableToDataset(table, adapter.Name, targetNS, tableURL)

		existing := &nairav1alpha1.Dataset{}
		err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
		if errors.IsNotFound(err) {
			if createErr := r.Create(ctx, &desired); createErr != nil {
				return 0, fmt.Errorf("creating Dataset %s: %w", desired.Name, createErr)
			}
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("getting Dataset %s: %w", desired.Name, err)
		}

		// Update spec if it differs.
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		if updateErr := r.Update(ctx, existing); updateErr != nil {
			return 0, fmt.Errorf("updating Dataset %s: %w", existing.Name, updateErr)
		}
	}

	return len(tables), nil
}

// resolveToken fetches the API token from the Kubernetes Secret referenced by
// the adapter's AuthSecretRef. Credentials are handled in-memory only.
func (r *Reconciler) resolveToken(ctx context.Context, adapter *nairav1alpha1.OpenMetadataAdapter) (string, error) {
	ref := adapter.Spec.AuthSecretRef

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: adapter.Namespace}, secret); err != nil {
		return "", fmt.Errorf("fetching secret %s/%s: %w", adapter.Namespace, ref.Name, err)
	}

	key := ref.Key
	if key == "" {
		key = "token"
	}

	raw, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain key %q", adapter.Namespace, ref.Name, key)
	}

	return string(raw), nil
}

// setCondition updates the Synced condition on the adapter's status and
// persists it. Errors during the status update are logged but not returned to
// avoid masking the original error.
func (r *Reconciler) setCondition(
	ctx context.Context,
	adapter *nairav1alpha1.OpenMetadataAdapter,
	status metav1.ConditionStatus,
	reason, message string,
) {
	logger := log.FromContext(ctx)
	condition := metav1.Condition{
		Type:               conditionTypeSynced,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// Replace any existing Synced condition.
	found := false
	for i, c := range adapter.Status.Conditions {
		if c.Type == conditionTypeSynced {
			adapter.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		adapter.Status.Conditions = append(adapter.Status.Conditions, condition)
	}

	if err := r.Status().Update(ctx, adapter); err != nil {
		logger.Error(err, "failed to update adapter status")
	}
}
