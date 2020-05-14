package chaosscheduler

import (
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ref "k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorV1 "github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	schedulerV1 "github.com/litmuschaos/chaos-scheduler/pkg/apis/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/controller/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func schedule(schedulerReconcile *reconcileScheduler, scheduler *chaosTypes.SchedulerInfo) (reconcile.Result, error) {

	schedulerReconcile.reqLogger.Info("Current scheduler type derived is ", "schedulerType", scheduler.Instance.Spec.Schedule.Type)
	switch scheduler.Instance.Spec.Schedule.Type {
	case "now":
		{
			return schedulerReconcile.createForNowAndOnce(scheduler)
		}
	case "once":
		{
			scheduleTime := time.Now().Add(time.Minute * time.Duration(1))
			startDuration := scheduler.Instance.Spec.Schedule.ExecutionTime.Local().Sub(scheduleTime)

			if startDuration.Seconds() < 0 {
				return schedulerReconcile.createForNowAndOnce(scheduler)
			}
			schedulerReconcile.reqLogger.Info("Time left to schedule the cronjob", "Duration", scheduleTime.Sub(scheduler.Instance.Spec.Schedule.StartTime.Local()))
			return reconcile.Result{RequeueAfter: startDuration}, nil
		}
	case "repeat":
		{
			/* StartDuration is the duration between current time
			 * and the scheduled time to start the chaos which is
			 * being used by reconciler to reque this resource after
			 * that much duration
			 * Chaos is being started 1 min before the scheduled time
			 */
			scheduleTime := time.Now()
			startDuration := scheduler.Instance.Spec.Schedule.StartTime.Local().Sub(scheduleTime)

			if startDuration.Seconds() < 0 {
				return schedulerReconcile.createEngineRepeat(scheduler)
			}
			schedulerReconcile.reqLogger.Info("Time left to schedule the cronjob", "Duration", scheduleTime.Sub(scheduler.Instance.Spec.Schedule.StartTime.Local()))
			return reconcile.Result{RequeueAfter: startDuration}, nil
		}
	}
	return reconcile.Result{}, errors.New("ScheduleType should be one of ('now', 'once', 'repeat')")
}

func (r *ReconcileChaosScheduler) getRef(object runtime.Object) (*corev1.ObjectReference, error) {
	return ref.GetReference(r.scheme, object)
}

// getEngineFromTemplate makes an Engine from a Schedule
func getEngineFromTemplate(cs *chaosTypes.SchedulerInfo) *operatorV1.ChaosEngine {

	labels := map[string]string{
		"app":      "chaos-engine",
		"chaosUID": string(cs.Instance.UID),
	}

	engine := &operatorV1.ChaosEngine{}

	ownerReferences := make([]metav1.OwnerReference, 0)
	ownerReferences = append(ownerReferences, *metav1.NewControllerRef(cs.Instance, schedulerV1.SchemeGroupVersion.WithKind("ChaosSchedule")))
	engine.SetOwnerReferences(ownerReferences)
	engine.SetLabels(labels)

	engine.Spec = cs.Instance.Spec.EngineTemplateSpec

	return engine
}
