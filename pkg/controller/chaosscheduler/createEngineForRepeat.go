package chaosscheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	cron "github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	schedulerV1 "github.com/litmuschaos/chaos-scheduler/pkg/apis/litmuschaos/v1alpha1"
	chaosTypes "github.com/litmuschaos/chaos-scheduler/pkg/controller/types"
)

func (schedulerReconcile *reconcileScheduler) createEngineRepeat(cs *chaosTypes.SchedulerInfo) (reconcile.Result, error) {

	err := schedulerReconcile.r.updateActiveStatus(cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	if errUpdate := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		schedulerReconcile.reqLogger.Error(errUpdate, "error updating status")
		return reconcile.Result{}, errUpdate
	}

	if metav1.Now().After(cs.Instance.Spec.Schedule.Repeat.EndTime.Time) {

		schedulerReconcile.reqLogger.Info("end time already passed", "endTime", cs.Instance.Spec.Schedule.Repeat.EndTime)
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
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedNeedsStart", "Cannot determine if engine needs to be started: %v", errNew)
		return reconcile.Result{}, errNew
	}

	if scheduledTime == nil {
		schedulerReconcile.reqLogger.Info("not found any scheduled time, reconciling after 10 seconds")
		return reconcile.Result{RequeueAfter: time.Second * 10}, nil
	}
	// TODO: set the concurencyPolicy and add the  different cases to be handled
	// For now taking "Forbid" as by default
	// if cs.Instance.Spec.ConcurrencyPolicy == schedulerV1.ForbidConcurrent && len(cs.Instance.Status.Active) > 0 {
	if len(cs.Instance.Status.Active) > 0 {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "MissEngine", "Missed scheduled time to start an engine because of an active engine at: %s", scheduledTime.Format(time.RFC1123Z))
		durationForNextScheduledTime := scheduledTime.Sub(time.Now())
		return reconcile.Result{RequeueAfter: durationForNextScheduledTime}, nil
	}

	_, err = schedulerReconcile.createNewEngine(cs, *scheduledTime)
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

	errCreate := schedulerReconcile.r.client.Create(context.TODO(), engineReq)
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

	cs.Instance.Spec.ScheduleState = schedulerV1.StateActive
	cs.Instance.Status.Schedule.Status = schedulerV1.StatusRunning
	ref, errRef := schedulerReconcile.r.getRef(engineReq)
	if errRef != nil {
		schedulerReconcile.reqLogger.Error(errRef, "Unable to make object reference for ", "engine", engineReq.Name)
	} else {
		cs.Instance.Status.Active = append(cs.Instance.Status.Active, *ref)
	}
	cs.Instance.Status.LastScheduleTime = &metav1.Time{Time: scheduledTime}
	cs.Instance.Status.Schedule.RunInstances = cs.Instance.Status.Schedule.RunInstances + 1
	cs.Instance.Status.Schedule.StartTime = cs.Instance.Spec.Schedule.Repeat.StartTime

	if errUpdate := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		return reconcile.Result{}, errUpdate
	}

	return reconcile.Result{}, nil
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
		earliestTime = cs.Instance.Spec.Schedule.Repeat.StartTime.Time.Add(time.Minute * -1)
	}

	if earliestTime.After(now) {
		return nil, nil
	}
	var previousTime *time.Time

	for t := sched.Next(earliestTime); !t.After(now); t = sched.Next(t) {
		temp := t
		previousTime = &temp
	}
	// cs.Instance.Status.Schedule.ExpectedNextRunTime = metav1.Time{Time: sched.Next(*previousTime)}
	return previousTime, nil
}

func scheduleRepeat(cs *chaosTypes.SchedulerInfo) (string, time.Duration, error) {

	interval, err := fetchInterval(cs.Instance.Spec.Schedule.Repeat.MinChaosInterval)
	if err != nil {
		return "", time.Duration(0), errors.New("error in parsing minChaosInterval(make sure to include 'm' or 'h' suffix for minutes and hours respectively)")
	}
	instances, err := fetchInstances(cs.Instance.Spec.Schedule.Repeat.InstanceCount)
	if err != nil {
		return "", time.Duration(0), errors.New("error in parsing instanceCount")
	}

	startTime := cs.Instance.Spec.Schedule.Repeat.StartTime
	endTime := cs.Instance.Spec.Schedule.Repeat.EndTime
	/* includedDays will be given in form comma seperated
	 * list such as 0,2,4 or Mon,Wed,Sat
	 * or in the range form such as 2-4 or Mon-Wed
	 * 0 represents Sunday and 6 represents Saturday
	 */
	includedDays := cs.Instance.Spec.Schedule.Repeat.IncludedDays
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
		if strings.Contains(cs.Instance.Spec.Schedule.Repeat.MinChaosInterval, "m") {
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
			cs.Instance.Spec.Schedule.Repeat.MinChaosInterval = fmt.Sprintf("%dh", int(intervalHours))
			return fmt.Sprintf("* */%d * * %s", int(intervalHours), includedDays), time.Hour * time.Duration(intervalHours), nil
		} else if intervalMinutes >= 1 {
			// to be sent in form of EnvVariable to executor
			cs.Instance.Spec.Schedule.Repeat.MinChaosInterval = fmt.Sprintf("%dm", int(intervalMinutes))
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

// getTimeHash returns Unix Epoch Time
func getTimeHash(scheduledTime time.Time) int64 {
	return scheduledTime.Unix()
}
