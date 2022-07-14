/*
Copyright 2019 LitmusChaos Authors

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

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	schedulerV1 "github.com/litmuschaos/chaos-scheduler/api/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/types"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	litmuschaosiov1alpha1 "github.com/litmuschaos/chaos-scheduler/api/litmuschaos/v1alpha1"
)

// ChaosScheduleReconciler reconciles a ChaosSchedule object
type ChaosScheduleReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client.Client
	Scheme *runtime.Scheme
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	Recorder record.EventRecorder
}

// reconcileScheduler contains details of reconcileScheduler
type reconcileScheduler struct {
	r         *ChaosScheduleReconciler
	reqLogger logr.Logger
}

//+kubebuilder:rbac:groups=litmuschaos.io,resources=chaosschedules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=litmuschaos.io,resources=chaosschedules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=litmuschaos.io,resources=chaosschedules/finalizers,verbs=update

/*Reconcile reads that state of the cluster for a ChaosScheduler object and makes changes based on the state read
and what is in the ChaosScheduler.Spec
Note:
The Controller will requeue the Request to be processed again if the returned error is non-nil or
Result.Requeue is true, otherwise upon completion it will remove the work from the queue.*/
func (r *ChaosScheduleReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {

	reqLogger := chaosTypes.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ChaosScheduler")

	// Fetch the ChaosScheduler instance
	scheduler, err := r.getChaosSchedulerInstance(request)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	schedulerReconcile := &reconcileScheduler{
		r:         r,
		reqLogger: reqLogger,
	}

	switch scheduler.Instance.Spec.ScheduleState {
	case "", schedulerV1.StateActive:
		return schedulerReconcile.reconcileForCreationAndRunning(scheduler, request)
	case schedulerV1.StateCompleted:
		if !checkScheduleStatus(scheduler, schedulerV1.StatusCompleted) {
			return schedulerReconcile.reconcileForComplete(scheduler, request)
		}
	case schedulerV1.StateHalted:
		if !checkScheduleStatus(scheduler, schedulerV1.StatusHalted) {
			return schedulerReconcile.reconcileForHalt(scheduler, request)
		}
	}
	return reconcile.Result{}, nil
}

func (schedulerReconcile *reconcileScheduler) reconcileForHalt(cs *chaosTypes.SchedulerInfo, request reconcile.Request) (reconcile.Result, error) {

	cs.Instance.Status.Schedule.Status = schedulerV1.StatusHalted
	if errUpdate := schedulerReconcile.r.Client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		schedulerReconcile.r.Recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "ScheduleHalted", "Cannot update status as halted")
		schedulerReconcile.reqLogger.Error(errUpdate, "error updating status")
		return reconcile.Result{}, errUpdate
	}
	schedulerReconcile.r.Recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "ScheduleHalted", "Schedule halted successfully")
	return reconcile.Result{}, nil
}
func (schedulerReconcile *reconcileScheduler) reconcileForComplete(cs *chaosTypes.SchedulerInfo, request reconcile.Request) (reconcile.Result, error) {

	if len(cs.Instance.Status.Active) != 0 {
		errUpdate := schedulerReconcile.r.updateActiveStatus(cs)
		if errUpdate != nil {
			return reconcile.Result{}, errUpdate
		}
		return reconcile.Result{}, nil
	}

	opts := client.UpdateOptions{}
	cs.Instance.Status.Schedule.Status = schedulerV1.StatusCompleted
	cs.Instance.Status.Schedule.EndTime = &metav1.Time{Time: time.Now()}
	if err := schedulerReconcile.r.Client.Update(context.TODO(), cs.Instance, &opts); err != nil {
		schedulerReconcile.r.Recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "ScheduleCompleted", "Cannot update status as completed")
		return reconcile.Result{}, fmt.Errorf("unable to update chaosSchedule for status completed, due to error: %v", err)
	}
	schedulerReconcile.r.Recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "ScheduleCompleted", "Schedule completed successfully")
	return reconcile.Result{}, nil
}

func (schedulerReconcile *reconcileScheduler) reconcileForCreationAndRunning(cs *chaosTypes.SchedulerInfo, request reconcile.Request) (reconcile.Result, error) {

	reconcileRes, err := schedule(schedulerReconcile, cs, request)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcileRes, nil
}

func checkScheduleStatus(cs *chaosTypes.SchedulerInfo, status schedulerV1.ChaosStatus) bool {
	return cs.Instance.Status.Schedule.Status == status
}

// Fetch the ChaosScheduler instance
func (r *ChaosScheduleReconciler) getChaosSchedulerInstance(request reconcile.Request) (*chaosTypes.SchedulerInfo, error) {
	instance := &schedulerV1.ChaosSchedule{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		// Error reading the object - requeue the request.
		return nil, err
	}
	scheduler := &chaosTypes.SchedulerInfo{
		Instance: instance,
	}
	return scheduler, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChaosScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&litmuschaosiov1alpha1.ChaosSchedule{}).
		Owns(&v1alpha1.ChaosEngine{}).
		Complete(r)
}
