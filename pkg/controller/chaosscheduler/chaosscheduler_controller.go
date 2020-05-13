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
	"k8s.io/apimachinery/pkg/types"
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

const finalizer = "chaosschedule.litmuschaos.io/finalizer"

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
		{
			return schedulerReconcile.reconcileForCreationAndRunning(scheduler)
		}
	// case schedulerV1.StateStopped:
	// 	{
	// 		if !checkScheduleStatus(scheduler, schedulerV1.StatusStopped) {
	// 			return r.reconcileForDelete(scheduler, request)
	// 		}
	// 	}
	case schedulerV1.StateCompleted:
		{
			if !checkScheduleStatus(scheduler, schedulerV1.StatusCompleted) {
				return schedulerReconcile.reconcileForComplete(scheduler, request)
			}
		}
	case schedulerV1.StateHalted:
		{
			if !checkScheduleStatus(scheduler, schedulerV1.StatusHalted) {
				return schedulerReconcile.reconcileForHalt(scheduler, request)
			}
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
	cs.Instance.Status.Schedule.EndTime = metav1.Now()
	if err := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance, &opts); err != nil {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "ScheduleCompleted", "Cannot update status as completed")
		return reconcile.Result{}, fmt.Errorf("Unable to update chaosSchedule for status completed, due to error: %v", err)
	}
	schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "ScheduleCompleted", "Schedule completed successfully")
	return reconcile.Result{}, nil
}

func (schedulerReconcile *reconcileScheduler) reconcileForCreationAndRunning(cs *chaosTypes.SchedulerInfo) (reconcile.Result, error) {

	reconcileRes, err := schedule(schedulerReconcile, cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcileRes, nil
}

func checkScheduleStatus(cs *chaosTypes.SchedulerInfo, status schedulerV1.ChaosStatus) bool {
	return cs.Instance.Status.Schedule.Status == status
}

func (schedulerReconcile *reconcileScheduler) createEngineRepeat(cs *chaosTypes.SchedulerInfo) (reconcile.Result, error) {

	err := schedulerReconcile.r.updateActiveStatus(cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	if errUpdate := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		schedulerReconcile.reqLogger.Error(errUpdate, "error updating status")
		return reconcile.Result{}, errUpdate
	}

	if metav1.Now().After(cs.Instance.Spec.Schedule.EndTime.Time) {

		schedulerReconcile.reqLogger.Info("end time already passed", "endTime", cs.Instance.Spec.Schedule.EndTime)
		cs.Instance.Spec.ScheduleState = schedulerV1.StateCompleted
		if errUpdate := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); errUpdate != nil {
			return reconcile.Result{}, errUpdate
		}
		return reconcile.Result{}, nil
	}

	if cs.Instance.DeletionTimestamp != nil {
		// The Schedule is being deleted.
		// Don't do anything other than updating status.
		return reconcile.Result{}, nil
	}

	cronString, duration, err := scheduleRepeat(cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	scheduledTime, errNew := getRecentUnmetScheduleTime(cs, cronString)
	if errNew != nil {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedNeedsStart", "Cannot determine if engine needs to be started: %v", errNew)		return reconcile.Result{}, errNew
	}

	if scheduledTime == nil {
		schedulerReconcile.reqLogger.Info("not found any scheduled time", "reconciling after", duration.Minutes())
		return reconcile.Result{RequeueAfter: duration}, nil
	}

	// TODO: set the concurencyPolicy and add the  different cases to be handled
	// For now taking "Forbid" as by default
	// if cs.Instance.Spec.ConcurrencyPolicy == schedulerV1.ForbidConcurrent && len(cs.Instance.Status.Active) > 0 {
	if len(cs.Instance.Status.Active) > 0 {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "MissEngine", "Missed scheduled time to start an engine because of an active engine at: %s", scheduledTime.Format(time.RFC1123Z))
		return reconcile.Result{RequeueAfter: duration}, nil
	}

	_, err = schedulerReconcile.r.createNewEngine(cs, *scheduledTime)
	if err != nil {
		return reconcile.Result{}, err
	}
	schedulerReconcile.reqLogger.Info("Will Reconcile later", "after", duration.Minutes())
	return reconcile.Result{RequeueAfter: duration}, nil
}

func (schedulerReconcile *reconcileScheduler) createNewEngine(cs *chaosTypes.SchedulerInfo, scheduledTime time.Time) (reconcile.Result, error) {

	engineReq := getEngineFromTemplate(cs)
	engineReq.Name = fmt.Sprintf("%s-%d", cs.Instance.Name, getTimeHash(scheduledTime))
	engineReq.Namespace = cs.Instance.Namespace

	errCreate := r.client.Create(context.TODO(), engineReq)
	if errCreate != nil {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedCreate", "Error creating engine: %v", errCreate)
		return reconcile.Result{}, errCreate
	}
	schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "SuccessfulCreate", "Created engine %v", engineReq.Name)

	// ------------------------------------------------------------------ //

	// If this process restarts at this point (after posting a engine, but
	// before updating the status), then we might try to start the engine on
	// the next time.  Actually, if we re-list the Engines on the next
	// iteration of Reconcile loop, we might not see our own status update, and
	// then post one again.  So, we need to use the engine name as a lock to
	// prevent us from making the engine twice (name the engine with hash of its
	// scheduled time).

	cs.Instance.Status.Schedule.Status = schedulerV1.StatusRunning
	ref, errRef := schedulerReconcile.r.getRef(engineReq)
	if errRef != nil {
		schedulerReconcile.reqLogger.Error(errRef, "Unable to make object reference for ", "engine", engineReq.Name)
	} else {
		cs.Instance.Status.Active = append(cs.Instance.Status.Active, *ref)
	}
	cs.Instance.Status.LastScheduleTime = &metav1.Time{Time: metav1.Now().Time}
	cs.Instance.Status.Schedule.RunInstances = cs.Instance.Status.Schedule.RunInstances + 1

	if errUpdate := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		return reconcile.Result{}, errUpdate
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileChaosScheduler) updateActiveStatus(cs *chaosTypes.SchedulerInfo) error {
	optsList := []client.ListOption{
		client.InNamespace(cs.Instance.Namespace),
		client.MatchingLabels{
			"app":      "chaos-engine",
			"chaosUID": string(cs.Instance.UID)},
	}

	var engineList operatorV1.ChaosEngineList
	if errList := r.client.List(context.TODO(), &engineList, optsList...); errList != nil {
		return errList
	}

	childrenJobs := make(map[types.UID]bool)
	for _, j := range engineList.Items {
		childrenJobs[j.ObjectMeta.UID] = true
		found := inActiveList(*cs, j.ObjectMeta.UID)

		if found && IsEngineFinished(&j) {
			deleteFromActiveList(cs, j.ObjectMeta.UID)
			r.recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "SawCompletedEngine", "Saw completed engine: %s, status: %v", j.Name, operatorV1.EngineStatusCompleted)
		}
	}

	// Remove any engine reference from the active list if the corresponding engine does not exist any more.
	// Otherwise, the schedule may be stuck in active mode forever even though there is no matching
	// engine running.
	for _, j := range cs.Instance.Status.Active {
		if found := childrenJobs[j.UID]; !found {
			r.recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "MissingEngine", "Active engine went missing: %v", j.Name)
			deleteFromActiveList(cs, j.UID)
		}
	}

	return nil
}
