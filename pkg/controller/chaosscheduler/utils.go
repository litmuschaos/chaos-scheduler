package chaosscheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	operatorV1 "github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	schedulerV1 "github.com/litmuschaos/chaos-scheduler/pkg/apis/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/controller/types"
	cron "github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ref "k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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

// getTimeHash returns Unix Epoch Time
func getTimeHash(scheduledTime time.Time) int64 {
	return scheduledTime.Unix()
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

func getRecentUnmetScheduleTime(cs *chaosTypes.SchedulerInfo, cronString string) (*time.Time, error) {

	now := time.Now()
	sched, err := cron.ParseStandard(cronString)
	if err != nil {
		return nil, fmt.Errorf("unparseable schedule: %s : %s", cronString, err)
	}

	var earliestTime time.Time
	if cs.Instance.Status.LastScheduleTime != nil {
		earliestTime = cs.Instance.Status.LastScheduleTime.Time
	} else {
		// If none found, then this is either a recently created schedule,
		// or the active/completed info was somehow lost (contract for status
		// in kubernetes says it may need to be recreated), or that we have
		// started a engine, but have not noticed it yet (distributed systems can
		// have arbitrary delays).  In any case, use the creation time of the
		// Schedule as last known start time.
		earliestTime = cs.Instance.Spec.Schedule.StartTime.Time
	}
	if earliestTime.After(now) {
		return nil, nil
	}

	var previousTime *time.Time

	for t := sched.Next(earliestTime); !t.After(now); t = sched.Next(t) {

		previousTime = &t
	}
	return previousTime, nil
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

func scheduleRepeat(cs *chaosTypes.SchedulerInfo) (string, time.Duration, error) {

	interval, err := fetchInterval(cs.Instance.Spec.Schedule.MinChaosInterval)
	if err != nil {
		return "", time.Duration(0), errors.New("error in parsing minChaosInterval(make sure to include 'm' or 'h' suffix for minutes and hours respectively)")
	}
	instances, err := fetchInstances(cs.Instance.Spec.Schedule.InstanceCount)
	if err != nil {
		return "", time.Duration(0), errors.New("error in parsing instanceCount")
	}

	startTime := cs.Instance.Spec.Schedule.StartTime
	endTime := cs.Instance.Spec.Schedule.EndTime
	/* includedDays will be given in form comma seperated
	 * list such as 0,2,4 or Mon,Wed,Sat
	 * or in the range form such as 2-4 or Mon-Wed
	 * 0 represents Sunday and 6 represents Saturday
	 */
	includedDays := cs.Instance.Spec.Schedule.IncludedDays
	if includedDays == "" {
		return "", time.Duration(0), errors.New("Missing IncludedDays")
	}
	duration := endTime.Sub(startTime.UTC())
	// One of the minChaosInterval or instances is mandatory to be given
	if interval != 0 {
		/* MinChaosInterval will be in form of "10m" or "2h"
		 * where 'm' or 'h' indicating "minutes" or "hours" respectively
		 */
		// cs.Instance.Status.Schedule.TotalInstances, err = getTotalInstances(cs)
		// if err != nil {
		// 	return "", err
		// }
		if strings.Contains(cs.Instance.Spec.Schedule.MinChaosInterval, "m") {
			return fmt.Sprintf("*/%d * * * %s", interval, includedDays), time.Minute * time.Duration(interval), nil
		}
		return fmt.Sprintf("* */%d * * %s", interval, includedDays), time.Hour * time.Duration(interval), nil
	} else if instances != 0 {
		cs.Instance.Status.Schedule.TotalInstances = instances
		//schedule at the end time will not be able to schedule so increaing the no. of instance
		intervalHours := duration.Hours() / float64(instances)
		intervalMinutes := duration.Minutes() / float64(instances)

		if intervalHours >= 1 {
			// to be sent in form of EnvVariable to executor
			cs.Instance.Spec.Schedule.MinChaosInterval = fmt.Sprintf("%dh", int(intervalHours))
			return fmt.Sprintf("* */%d * * %s", int(intervalHours), includedDays), time.Hour * time.Duration(intervalHours), nil
		} else if intervalMinutes >= 1 {
			// to be sent in form of EnvVariable to executor
			cs.Instance.Spec.Schedule.MinChaosInterval = fmt.Sprintf("%dm", int(intervalMinutes))
			return fmt.Sprintf("*/%d * * * %s", int(intervalMinutes), includedDays), time.Minute * time.Duration(intervalMinutes), nil
		}
		return "", time.Duration(0), errors.New("Too many instances to execute")
	}
	return "", time.Duration(0), errors.New("MinChaosInterval and InstanceCount both not found")
}

func fetchInterval(minChaosInterval string) (int, error) {
	/* MinChaosInterval will be in form of "10m" or "2h"
	 * where 'm' or 'h' indicating "minutes" or "hours" respectively
	 */
	if minChaosInterval == "" {
		return 0, nil
	} else if strings.Contains(minChaosInterval, "h") {
		return strconv.Atoi(strings.Split(minChaosInterval, "h")[0])
	} else if strings.Contains(minChaosInterval, "m") {
		return strconv.Atoi(strings.Split(minChaosInterval, "m")[0])
	}
	return 0, errors.New("minChaosInterval should be in either minutes or hours and the prefix should be 'm' or 'h' respectively")
}

func fetchInstances(instanceCount string) (int, error) {
	if instanceCount == "" {
		return 0, nil
	}
	return strconv.Atoi(instanceCount)
}
