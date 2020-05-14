package chaosscheduler

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorV1 "github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/controller/types"
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
		}
	}

	return nil
}

// deleteJob reaps a job, deleting the job, the pods and the reference in the active list
func (r *ReconcileChaosScheduler) deleteEngine(cs *chaosTypes.SchedulerInfo, engine *operatorV1.ChaosEngine) bool {

	// delete the engine itself...
	err := r.client.Delete(context.TODO(), engine, []client.DeleteOption{}...)
	if err != nil {
		r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedDelete", "Deleted engine: %v", err)
		return false
	}
	// ... and its reference from active list
	deleteFromActiveList(cs, engine.ObjectMeta.UID)
	r.recorder.Eventf(cs.Instance, corev1.EventTypeNormal, "SuccessfulDelete", "Deleted engine %v", engine.Name)

	return true
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
