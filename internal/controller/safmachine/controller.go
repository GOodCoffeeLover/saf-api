/*
Copyright 2025 GoodCoffeeLover.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package safmachine

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/GoodCoffeeLover/saf-api/api/v1alpha1"
)

// Reconciler reconciles a SAFMachine object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.SAFMachine{}).
		Named("safmachine").
		Complete(r)
}

// +kubebuilder:rbac:groups=infrastructure.saf-api.io,resources=safmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.saf-api.io,resources=safmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.saf-api.io,resources=safmachines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	safm := &infrastructurev1alpha1.SAFMachine{}
	if err := r.Get(ctx, req.NamespacedName, safm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(fmt.Errorf("get saf machine: %w", err))
	}

	ma, err := util.GetOwnerMachine(ctx, r.Client, safm.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get owner machine: %w", err)
	}

	if ma == nil {
		// will requeue on update
		return ctrl.Result{}, nil
	}

	if ma.Spec.Bootstrap.DataSecretName == nil {
		// will requeue on update
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, err
}
