// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package controller contains the core Dataset reconciler. Unlike the adapter
// controllers (e.g., openmetadata), this controller is deliberately
// tool-agnostic: it operates only on generic Dataset resources and knows
// nothing about where the data originated.
package controller

import (
	"context"

	nairav1alpha1 "github.com/naira-project/naira/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DatasetReconciler reconciles Dataset resources. Its primary responsibility is
// lifecycle management: it can enforce invariants, clean up stale resources,
// and emit events. It never calls any external data catalog API.
type DatasetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager registers the DatasetReconciler with the manager.
func (r *DatasetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nairav1alpha1.Dataset{}).
		Complete(r)
}

// Reconcile is called whenever a Dataset resource changes. It currently
// performs a no-op reconciliation, logging the event. Future versions will
// enforce retention policies, validate schemas, and emit Kubernetes Events.
func (r *DatasetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dataset := &nairav1alpha1.Dataset{}
	if err := r.Get(ctx, req.NamespacedName, dataset); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciling Dataset",
		"name", dataset.Name,
		"namespace", dataset.Namespace,
		"sourceSystem", dataset.Spec.SourceSystem,
	)

	return ctrl.Result{}, nil
}
