package safmachine

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

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

		if err := r.Get(ctx, provJobKey, provJob); errors.IsNotFound(err) {
			l.Info("provision job not found", "provision_job_name", provJobKey.Name)
			return r.createProvisionJob(ctx, s)
		} else if err != nil {
			return ctrl.Result{}, err
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

	provisionJob.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	provisionJob.Spec.BackoffLimit = ptr.To[int32](1)

	if err := controllerutil.SetControllerReference(s.safMachine, provisionJob, r.Scheme,
		controllerutil.WithBlockOwnerDeletion(true)); err != nil {
		return ctrl.Result{}, fmt.Errorf("set controller ref before create: %w", err)
	}

	return ctrl.Result{}, r.Create(ctx, provisionJob)
}
