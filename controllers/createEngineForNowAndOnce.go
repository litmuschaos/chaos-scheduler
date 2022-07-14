package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorV1 "github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	schedulerV1 "github.com/litmuschaos/chaos-scheduler/api/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/types"
)

func (schedulerReconcile *reconcileScheduler) createForNowAndOnce(cs *chaosTypes.SchedulerInfo, request reconcile.Request) (reconcile.Result, error) {

	err := schedulerReconcile.r.updateActiveStatus(cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	if errUpdate := schedulerReconcile.r.Client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		return reconcile.Result{}, errUpdate
	}

	currentTime := metav1.Now()
	engine := &operatorV1.ChaosEngine{}
	err = schedulerReconcile.r.Client.Get(context.TODO(), types.NamespacedName{Name: cs.Instance.Name, Namespace: cs.Instance.Namespace}, engine)
	if err != nil && k8serrors.IsNotFound(err) {
		schedulerReconcile.reqLogger.Info("Creating a new engine", "Pod.Namespace", cs.Instance.Name, "Pod.Name", cs.Instance.Namespace)

		engine, err = schedulerReconcile.r.getEngineFromTemplate(cs)
		if err != nil {
			schedulerReconcile.r.Recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedCreate", "Failed to add controller references: %v", err)
			return reconcile.Result{}, err
		}

		if err = schedulerReconcile.r.Client.Create(context.TODO(), engine); err != nil {
			schedulerReconcile.r.Recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedCreate", "Error creating engine: %v", err)
			return reconcile.Result{}, err
		}
		schedulerReconcile.r.Recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "SuccessfulCreate", "Created engine %v", engine.Name)
		cs.Instance.Spec.ScheduleState = schedulerV1.StateActive
		cs.Instance.Status.Schedule.Status = schedulerV1.StatusRunning
		cs.Instance.Status.Schedule.StartTime = &currentTime
		cs.Instance.Status.LastScheduleTime = &currentTime
		ref, errRef := schedulerReconcile.r.getRef(engine)
		if errRef != nil {
			return reconcile.Result{}, errRef
		}
		cs.Instance.Status.Active = append(cs.Instance.Status.Active, *ref)
		if err := schedulerReconcile.r.Client.Update(context.TODO(), cs.Instance); err != nil {
			return reconcile.Result{}, err
		}
		schedulerReconcile.reqLogger.Info("Engine created successfully")
	} else if err != nil {
		return reconcile.Result{}, err
	} else if IsEngineFinished(engine) {
		cs.Instance.Spec.ScheduleState = schedulerV1.StateCompleted
		cs.Instance.Status.Schedule.EndTime = &currentTime
		if err := schedulerReconcile.UpdateSchedulerStatus(cs, request); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
