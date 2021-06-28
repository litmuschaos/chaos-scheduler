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

func (schedulerReconcile *reconcileScheduler) createEngineRepeat(cs *chaosTypes.SchedulerInfo, request reconcile.Request) (reconcile.Result, error) {

	err := schedulerReconcile.r.updateActiveStatus(cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := schedulerReconcile.UpdateSchedulerStatus(cs, request); err != nil {
		schedulerReconcile.reqLogger.Error(err, "error updating status")
		return reconcile.Result{}, err
	}

	timeRange := cs.Instance.Spec.Schedule.Repeat.TimeRange
	if timeRange != nil {
		endTime := timeRange.EndTime
		if endTime != nil && metav1.Now().After(endTime.Time) {

			schedulerReconcile.reqLogger.Info("end time already passed", "endTime", endTime)

			if err := schedulerReconcile.UpdateSchedulerStatus(cs, request); err != nil {
				schedulerReconcile.reqLogger.Error(err, "error updating status")
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
	}

	if cs.Instance.DeletionTimestamp != nil {
		// The Schedule is being deleted.
		// Don't do anything other than updating status.
		return reconcile.Result{}, nil
	}

	cronString, duration, err := schedulerReconcile.scheduleRepeat(cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	scheduledTime, wait, errNew := schedulerReconcile.getRecentUnmetScheduleTime(cs, cronString)
	if errNew != nil {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedNeedsStart", "Cannot determine if engine needs to be started: %v", errNew)
		return reconcile.Result{}, errNew
	}

	if timeRange != nil && time.Until(timeRange.EndTime.Time) < wait {
		schedulerReconcile.reqLogger.Info("no schedule time available before endtime", "endTime", timeRange.EndTime)
		if err := schedulerReconcile.UpdateSchedulerStatus(cs, request); err != nil {
			schedulerReconcile.reqLogger.Error(err, "error updating status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if wait > 0 {
		schedulerReconcile.reqLogger.Info("Hold on, time left to schedule the engine", "Duration(seconds)", wait.Seconds())
	}

	if scheduledTime == nil {
		return reconcile.Result{RequeueAfter: wait}, nil
	}

	// TODO: set the concurencyPolicy and add the  different cases to be handled
	// For now taking "Forbid" as by default
	if len(cs.Instance.Status.Active) > 0 {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "MissEngine", "Missed scheduled time to start an engine because of an active engine at: %s", scheduledTime.Format(time.RFC1123Z))
		return reconcile.Result{RequeueAfter: wait}, nil
	}

	_, err = schedulerReconcile.createNewEngine(cs, *scheduledTime)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: duration}, nil
}

func (schedulerReconcile *reconcileScheduler) createNewEngine(cs *chaosTypes.SchedulerInfo, scheduledTime time.Time) (reconcile.Result, error) {

	engineReq := getEngineFromTemplate(cs)
	engineReq.Name = fmt.Sprintf("%s-%d", cs.Instance.Name, getTimeHash(scheduledTime))

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

	var startTime *metav1.Time
	if cs.Instance.Spec.Schedule.Repeat.TimeRange != nil {
		startTime = cs.Instance.Spec.Schedule.Repeat.TimeRange.StartTime
	}

	if startTime == nil {
		startTime = &cs.Instance.CreationTimestamp
	}
	cs.Instance.Status.Schedule.StartTime = startTime

	if err := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (schedulerReconcile *reconcileScheduler) getRecentUnmetScheduleTime(cs *chaosTypes.SchedulerInfo, cronString string) (*time.Time, time.Duration, error) {

	now := time.Now()
	cronSchedule, err := cron.ParseStandard(cronString)
	if err != nil {
		return nil, 0, fmt.Errorf("unparseable schedule: %s : %s", cronString, err)
	}

	timeRange := cs.Instance.Spec.Schedule.Repeat.TimeRange
	var nextTime *time.Time
	var earliestTime time.Time

	if cs.Instance.Status.LastScheduleTime != nil {
		// handles all the schedules except first schedule
		earliestTime = cs.Instance.Status.LastScheduleTime.Time
	} else if timeRange != nil && timeRange.StartTime != nil && !cs.Instance.GetCreationTimestamp().Time.After(timeRange.StartTime.Time) {
		// handles the first schedule
		earliestTime = timeRange.StartTime.Time
	} else {
		// handles the first schedule
		earliestTime = cs.Instance.GetCreationTimestamp().Time
	}
	t := cronSchedule.Next(earliestTime)
	if !t.After(now) {
		nextTime = &now
	}
	wait := t.Sub(now)
	if wait < 0 {
		wait = 0
	}
	return nextTime, wait, nil
}

func (schedulerReconcile *reconcileScheduler) scheduleRepeat(cs *chaosTypes.SchedulerInfo) (string, time.Duration, error) {

	interval, err := fetchInterval(cs.Instance.Spec.Schedule.Repeat.Properties.MinChaosInterval)
	if err != nil {
		return "", time.Duration(0), errors.New("error in parsing minChaosInterval(make sure to include 'm' or 'h' suffix for minutes and hours respectively)")
	}

	/* includedDays will be given in form comma seperated
	 * list such as 0,2,4 or Mon,Wed,Sat
	 * or in the range form such as 2-4 or Mon-Wed
	 * 0 represents Sunday and 6 represents Saturday
	 */
	var includedDays string
	if cs.Instance.Spec.Schedule.Repeat.WorkDays != nil {
		includedDays = cs.Instance.Spec.Schedule.Repeat.WorkDays.IncludedDays
	} else {
		includedDays = "*"
	}

	var includedHours string
	if cs.Instance.Spec.Schedule.Repeat.WorkHours != nil {
		includedHours = cs.Instance.Spec.Schedule.Repeat.WorkHours.IncludedHours
	} else {
		includedHours = "*"
	}

	// One of the minChaosInterval or instances is mandatory to be given
	if interval != 0 {
		/* MinChaosInterval will be in form of "10m" or "2h"
		 * where 'm' or 'h' indicating "minutes" or "hours" respectively
		 */
		if strings.Contains(cs.Instance.Spec.Schedule.Repeat.Properties.MinChaosInterval, "m") {
			cron := fmt.Sprintf("*/%d %s * * %s", interval, includedHours, includedDays)
			schedulerReconcile.reqLogger.Info("CronString formed ", "Cron String", cron)
			return cron, time.Minute * time.Duration(interval), nil
		}
		cron := fmt.Sprintf("* %s/%d * * %s", includedHours, interval, includedDays)
		schedulerReconcile.reqLogger.Info("CronString formed ", "Cron String", cron)
		return cron, time.Hour * time.Duration(interval), nil
	}
	return "", time.Duration(0), errors.New("MinChaosInterval not found")
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

// getTimeHash returns Unix Epoch Time
func getTimeHash(scheduledTime time.Time) int64 {
	return scheduledTime.Unix()
}
