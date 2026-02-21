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
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	capv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/finalizers"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/GoodCoffeeLover/saf-api/api/v1alpha1"
)

// Reconciler reconciles a SAFMachine object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var controllerName = strings.ToLower(v1alpha1.SAFMachineKind)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	l := mgr.GetLogger().WithValues("controller", controllerName, "predicate", "true")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.SAFMachine{}).
		Owns(&batchv1.Job{}).
		Watches(
			&capv1beta2.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(v1alpha1.GroupVersion.WithKind(v1alpha1.SAFMachineKind))),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), l)),
		).
		Named(controllerName).
		Complete(r)
}

type scope struct {
	machine        *capv1beta2.Machine
	safMachine     *v1alpha1.SAFMachine
	provisionJob   *batchv1.Job
	deprovisionJob *batchv1.Job
}

// +kubebuilder:rbac:groups=infrastructure.saf-api.io,resources=safmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.saf-api.io,resources=safmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.saf-api.io,resources=safmachines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	safm := &v1alpha1.SAFMachine{}
	if err := r.Get(ctx, req.NamespacedName, safm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(fmt.Errorf("get saf machine: %w", err))
	}

	if changed, err := finalizers.EnsureFinalizer(ctx, r.Client, safm, v1alpha1.SAFMachineFinalizer); changed || err != nil {
		return ctrl.Result{}, err
	}

	ma, err := util.GetOwnerMachine(ctx, r.Client, safm.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get owner machine: %w", err)
	}

	s := &scope{
		safMachine: safm,
		machine:    ma,
	}
	pacher, err := patch.NewHelper(s.safMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("make patcher: %w", err)
	}
	defer func() {
		r.calculateStatus(ctx, s)
		opts := []patch.Option{
			patch.WithOwnedConditions{},
		}
		// Always attempt to patch the object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		if reterr == nil {
			opts = append(opts, patch.WithStatusObservedGeneration{})
		}
		if err := pacher.Patch(ctx, s.safMachine, opts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	phases := []reconcileFunc{
		r.findNode,
		r.provisionJob,
	}
	if s.safMachine.GetDeletionTimestamp() != nil {
		phases = append(phases, r.deprovisionJob)
	}

	return doReconcile(ctx, phases, s)
}

func doReconcile(ctx context.Context, phases []reconcileFunc, s *scope) (ctrl.Result, error) {
	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		// Call the inner reconciliation methods.
		phaseResult, err := phase(ctx, s)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		res = util.LowestNonZeroResult(res, phaseResult)
	}

	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}

	return res, nil
}

type reconcileFunc func(context.Context, *scope) (ctrl.Result, error)

func (r *Reconciler) findNode(ctx context.Context, s *scope) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *Reconciler) provisionJob(ctx context.Context, s *scope) (ctrl.Result, error) {
	// observe state
	l := logf.FromContext(ctx, "phase", "provisionJob")
	ctx = logf.IntoContext(ctx, l)

	{
		provJobKey := types.NamespacedName{
			Name:      fmt.Sprintf("%s-provision", s.safMachine.Name),
			Namespace: s.safMachine.Namespace,
		}
		provJob := &batchv1.Job{}

		if err := r.Get(ctx, provJobKey, provJob); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		} else if err != nil {
			l.Info("provision job not found", "provision_job_name", provJobKey.Name)
			return r.createProvisionJob(ctx, s)
		} else {
			s.provisionJob = provJob
		}
	}

	// ensure owned

	return ctrl.Result{}, nil
}

func (r *Reconciler) createProvisionJob(ctx context.Context, s *scope) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	// don't act, if machine deleting
	if s.safMachine.GetDeletionTimestamp() != nil {
		l.Info("safMachine is deleting")
		return ctrl.Result{}, nil
	}

	// act -- ensure there is succeeded provision job
	if s.machine == nil {
		// will requeue on update
		l.Info("safMachine's machine is not exsits")
		return ctrl.Result{}, nil
	}

	if s.machine.Spec.Bootstrap.DataSecretName == nil {
		// will requeue on update
		l.Info("safMachine's bootstrap is not prepared")
		return ctrl.Result{}, nil
	}

	provisionJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.safMachine.Name + "-provision",
			Namespace: s.safMachine.Namespace,
		},
		Spec: *s.safMachine.Spec.ProvisionJob.Spec.DeepCopy(),
	}

	provisionJob.Spec.Template.Spec.Volumes = append(provisionJob.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "bootstrap",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: *s.machine.Spec.Bootstrap.DataSecretName,
			},
		},
	})

	containers := provisionJob.Spec.Template.Spec.Containers
	for i := range containers {
		containers[i].VolumeMounts = append(containers[i].VolumeMounts, corev1.VolumeMount{
			Name:      "bootstrap",
			ReadOnly:  true,
			MountPath: "/etc/bootstrap/",
		})
	}

	if err := controllerutil.SetControllerReference(s.safMachine, provisionJob, r.Scheme,
		controllerutil.WithBlockOwnerDeletion(true)); err != nil {
		return ctrl.Result{}, fmt.Errorf("set controller ref before create: %w", err)
	}

	return ctrl.Result{}, r.Create(ctx, provisionJob)
}

func (r *Reconciler) deprovisionJob(ctx context.Context, s *scope) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *Reconciler) calculateStatus(ctx context.Context, s *scope) {
}
