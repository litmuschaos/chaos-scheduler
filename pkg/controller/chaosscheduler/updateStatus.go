package chaosscheduler

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorV1 "github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	schedulerV1 "github.com/litmuschaos/chaos-scheduler/pkg/apis/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/controller/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
)

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
			cs.Instance.Status.LastScheduleCompletionTime = &metav1.Time{Time: time.Now()}
		}
	}

	return nil
}

func deleteFromActiveList(cs *chaosTypes.SchedulerInfo, uid types.UID) {
	if cs == nil {
		return
	}
	newActive := []corev1.ObjectReference{}
	for _, j := range cs.Instance.Status.Active {
		if j.UID != uid {
			newActive = append(newActive, j)
		}
	}
	cs.Instance.Status.Active = newActive
}

func inActiveList(cs chaosTypes.SchedulerInfo, uid types.UID) bool {
	for _, j := range cs.Instance.Status.Active {
		if j.UID == uid {
			return true
		}
	}
	return false
}

// IsEngineFinished returns whether or not a job has completed successfully or failed.
func IsEngineFinished(j *operatorV1.ChaosEngine) bool {
	return j.Status.EngineStatus == operatorV1.EngineStatusCompleted
}

// UpdateSchedulerStatus updates the scheduler status for the complete
func (schedulerReconcile *reconcileScheduler) UpdateSchedulerStatus(cs *chaosTypes.SchedulerInfo, request reconcile.Request) error {
	cs.Instance.Status.Schedule.Status = schedulerV1.StatusCompleted
	cs.Instance.Status.Schedule.EndTime = &metav1.Time{Time: time.Now()}
	cs.Instance.Spec.ScheduleState = schedulerV1.StateCompleted
	cs.Instance.Status.Active = nil
	if err := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); k8serrors.IsConflict(err) {
		return retry.
			Times(uint(5)).
			Wait(1 * time.Second).
			Try(func(attempt uint) error {
				scheduler, err := schedulerReconcile.r.getChaosSchedulerInstance(request)
				if err != nil {
					return err
				}
				scheduler.Instance.Status.Schedule.Status = schedulerV1.StatusCompleted
				scheduler.Instance.Spec.ScheduleState = schedulerV1.StateCompleted
				scheduler.Instance.Status.Schedule.EndTime = &metav1.Time{Time: time.Now()}
				scheduler.Instance.Status.Active = nil
				return schedulerReconcile.r.client.Update(context.TODO(), scheduler.Instance)
			})
	}
	time.Sleep(1 * time.Second)
	return nil
}
