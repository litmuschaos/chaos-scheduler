package chaosscheduler

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorV1 "github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	schedulerV1 "github.com/litmuschaos/chaos-scheduler/pkg/apis/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/controller/types"
)

var _ reconcile.Reconciler = &ReconcileChaosScheduler{}

// ReconcileChaosScheduler reconciles a ChaosScheduler object
type ReconcileChaosScheduler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// reconcileScheduler contains details of reconcileScheduler
type reconcileScheduler struct {
	r         *ReconcileChaosScheduler
	reqLogger logr.Logger
}

// Add creates a new ChaosScheduler Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileChaosScheduler{client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetEventRecorderFor("chaos-scheduler")}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("chaosscheduler-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	err = watchChaosResources(mgr.GetClient(), c)
	if err != nil {
		return err
	}

	return nil
}

// watchSecondaryResources watch's for changes in chaos resources
func watchChaosResources(clientSet client.Client, c controller.Controller) error {
	// Watch for Primary Chaos Resource
	err := c.Watch(&source.Kind{Type: &schedulerV1.ChaosSchedule{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &operatorV1.ChaosEngine{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &schedulerV1.ChaosSchedule{},
		IsController: true,
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileChaosScheduler implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileChaosScheduler{}

/*Reconcile reads that state of the cluster for a ChaosScheduler object and makes changes based on the state read
and what is in the ChaosScheduler.Spec
Note:
The Controller will requeue the Request to be processed again if the returned error is non-nil or
Result.Requeue is true, otherwise upon completion it will remove the work from the queue.*/
func (r *ReconcileChaosScheduler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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
	if errUpdate := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "ScheduleHalted", "Cannot update status as halted")
		schedulerReconcile.reqLogger.Error(errUpdate, "error updating status")
		return reconcile.Result{}, errUpdate
	}
	schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "ScheduleHalted", "Schedule halted successfully")
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
	if err := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance, &opts); err != nil {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "ScheduleCompleted", "Cannot update status as completed")
		return reconcile.Result{}, fmt.Errorf("unable to update chaosSchedule for status completed, due to error: %v", err)
	}
	schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "ScheduleCompleted", "Schedule completed successfully")
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
func (r *ReconcileChaosScheduler) getChaosSchedulerInstance(request reconcile.Request) (*chaosTypes.SchedulerInfo, error) {
	instance := &schedulerV1.ChaosSchedule{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		// Error reading the object - requeue the request.
		return nil, err
	}
	scheduler := &chaosTypes.SchedulerInfo{
		Instance: instance,
	}
	return scheduler, nil
}
