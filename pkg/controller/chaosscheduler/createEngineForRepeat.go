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
	"github.com/litmuschaos/chaos-scheduler/pkg/controller/types"
)

func (schedulerReconcile *reconcileScheduler) createEngineRepeat(cs *types.SchedulerInfo, request reconcile.Request) (reconcile.Result, error) {

	err := schedulerReconcile.r.updateActiveStatus(cs)
	if err != nil {
		return reconcile.Result{}, err
	}

	if errUpdate := schedulerReconcile.r.client.Update(context.TODO(), cs.Instance); errUpdate != nil {
		schedulerReconcile.reqLogger.Error(errUpdate, "error updating status")
		return reconcile.Result{}, errUpdate
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

	scheduledTime, errNew := schedulerReconcile.getRecentUnmetScheduleTime(cs, cronString)
	if errNew != nil {
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "FailedNeedsStart", "Cannot determine if engine needs to be started: %v", errNew)
		return reconcile.Result{}, errNew
	}

	wait := time.Until(scheduledTime)

	if timeRange != nil && timeRange.EndTime != nil && time.Until(timeRange.EndTime.Time) < wait {
		return reconcile.Result{RequeueAfter: time.Until(timeRange.EndTime.Time)}, nil
	}

	if wait > 0 {
		schedulerReconcile.reqLogger.Info("Hold on, time left to schedule the engine", "Duration(seconds)", wait.Seconds())
		return reconcile.Result{RequeueAfter: wait}, nil
	}

	// TODO: set the concurencyPolicy and add the  different cases to be handled
	// For now taking "Forbid" as by default
	if len(cs.Instance.Status.Active) > 0 {
		schedulerReconcile.reqLogger.Info("The next scheduled is delayed as the older chaosengine is not completed yet")
		schedulerReconcile.r.recorder.Eventf(cs.Instance, corev1.EventTypeWarning, "MissEngine", "Missed scheduled time to start an engine because of an active engine at: %s", scheduledTime.Format(time.RFC1123Z))
		return reconcile.Result{RequeueAfter: wait}, nil
	}

	_, err = schedulerReconcile.createNewEngine(cs, scheduledTime)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: duration}, nil
}

func (schedulerReconcile *reconcileScheduler) createNewEngine(cs *types.SchedulerInfo, scheduledTime time.Time) (reconcile.Result, error) {

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
	time.Sleep(1 * time.Second)

	return reconcile.Result{}, nil
}

func (schedulerReconcile *reconcileScheduler) getRecentUnmetScheduleTime(cs *types.SchedulerInfo, cronString string) (time.Time, error) {

	now := time.Now()
	cronSchedule, err := cron.ParseStandard(cronString)
	if err != nil {
		return time.Time{}, fmt.Errorf("unparseable schedule: %s : %s", cronString, err)
	}
	timeRange := cs.Instance.Spec.Schedule.Repeat.TimeRange
	// handles all the schedules except first schedule
	if cs.Instance.Status.LastScheduleTime != nil {
		var previousTime *time.Time
		earliestTime := cs.Instance.Status.LastScheduleTime.Time
		for t := cronSchedule.Next(earliestTime); !t.After(now); t = cronSchedule.Next(t) {
			temp := t
			previousTime = &temp
		}
		if previousTime == nil {
			return cronSchedule.Next(earliestTime), nil
		}
		lastUpdatedTime := cs.Instance.Status.LastScheduleCompletionTime
		if lastUpdatedTime != nil {
			if lastUpdatedTime.Sub(*previousTime) >= 0 {
				return cronSchedule.Next(*previousTime), nil
			}
		}
		return *previousTime, nil
	} else if timeRange != nil && timeRange.StartTime != nil && !cs.Instance.GetCreationTimestamp().Time.After(timeRange.StartTime.Time) {
		return schedulerReconcile.firstScheduleTime(cs, timeRange.StartTime.Time, cronSchedule)
	} else {
		return schedulerReconcile.firstScheduleTime(cs, cs.Instance.GetCreationTimestamp().Time, cronSchedule)
	}
}

// it derive the first scheduled time for the scheduler based on the cron string
func (schedulerReconcile *reconcileScheduler) firstScheduleTime(cs *types.SchedulerInfo, earliestTime time.Time, cronSchedule cron.Schedule) (time.Time, error) {

	schedule := cs.Instance.Spec.Schedule.Repeat

	// it checks if present day is included in the includedWeekdays list
	if schedule.WorkDays != nil && schedule.WorkDays.IncludedDays != "" {
		isPossibleNow, err := isWeekdayPossible(schedule.WorkDays.IncludedDays)
		if err != nil {
			return time.Time{}, err
		}
		if !isPossibleNow {
			nextTime := cronSchedule.Next(earliestTime)
			if !nextTime.After(time.Now()) {
				nextTime = time.Now()
			}
			return nextTime, nil
		}
	}

	// it checks if current hour is included in the includedHours list
	if schedule.WorkHours != nil && schedule.WorkHours.IncludedHours != "" {
		isPossibleNow, err := isHoursPossible(schedule.WorkHours.IncludedHours)
		if err != nil {
			return time.Time{}, err
		}
		if !isPossibleNow {
			nextTime := cronSchedule.Next(earliestTime)
			if !nextTime.After(time.Now()) {
				nextTime = time.Now()
			}
			return nextTime, nil
		}
	}

	return time.Now(), nil
}

// it checks if provided week day is listed in the includeWeekdays list
func isWeekdayPossible(includedDays string) (bool, error) {
	finalDays := [7]int{}
	days := strings.Split(includedDays, ",")
	for _, d := range days {
		start, end, err := parseWeekdays(d)
		if err != nil {
			return false, err
		}
		for i := start; i <= end; i++ {
			finalDays[i] = 1
		}
	}

	currWeekday := time.Now().Weekday()
	if finalDays[currWeekday] == 1 {
		return true, nil
	}
	return false, nil
}

// it checks if provided hour is included in the includedHours list
func isHoursPossible(includedHours string) (bool, error) {
	finalHours := [24]int{}
	hours := strings.Split(includedHours, ",")
	for _, h := range hours {
		start, end, err := parseCronData(h)
		if err != nil {
			return false, err
		}
		for i := start; i <= end; i++ {
			finalHours[i] = 1
		}
	}

	currHour := time.Now().Hour()
	if finalHours[currHour] == 1 {
		return true, nil
	}
	return false, nil
}

// it parse the possible weekdays included in the inputs
func parseWeekdays(data string) (int, int, error) {
	var start, end int
	if strings.Contains(data, "-") {
		if len(data) <= 3 {
			return parseCronData(data)
		}
		fieldRange := strings.Split(data, "-")
		if len(fieldRange) < 2 {
			return 0, 0, fmt.Errorf("provided the correct input range, range: %v", data)
		}
		start = types.WeekDays[strings.ToLower(strings.TrimSpace(fieldRange[0]))]
		end = types.WeekDays[strings.ToLower(strings.TrimSpace(fieldRange[1]))]
	} else {
		if len(data) <= 1 {
			return parseCronData(data)
		}
		start = types.WeekDays[strings.ToLower(data)]
		end = types.WeekDays[strings.ToLower(data)]
	}
	return start, end, nil
}

// it parse the input range provided in int format
func parseCronData(data string) (int, int, error) {
	var start, end string
	if strings.Contains(data, "-") {
		fieldRange := strings.Split(data, "-")
		if len(fieldRange) < 2 {
			return 0, 0, fmt.Errorf("provided the correct input range, range: %v", data)
		}
		start = strings.TrimSpace(fieldRange[0])
		end = strings.TrimSpace(fieldRange[1])
	} else {
		start = data
		end = data
	}
	startInt, err := strconv.Atoi(start)
	if err != nil {
		return 0, 0, err
	}
	endInt, err := strconv.Atoi(end)
	if err != nil {
		return 0, 0, err
	}
	return startInt, endInt, nil
}

func (schedulerReconcile *reconcileScheduler) scheduleRepeat(cs *types.SchedulerInfo) (string, time.Duration, error) {

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
		cron := fmt.Sprintf("0 %s/%d * * %s", includedHours, interval, includedDays)
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
