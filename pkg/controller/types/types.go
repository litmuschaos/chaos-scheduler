// To create logs for debugging or detailing, please follow this syntax.
// use function log.Info
// in parameters give the name of the log / error (string) ,
// with the variable name for the value(string)
// and then the value to log (any datatype)
// All values should be in key : value pairs only
// For eg. : log.Info("name_of_the_log","variable_name_for_the_value",value, ......)
// For eg. : log.Error(err,"error_statement","variable_name",value)
// For eg. : log.Printf
//("error statement %q other variables %s/%s",targetValue, object.Namespace, object.Name)
// For eg. : log.Errorf
//("unable to reconcile object %s/%s: %v", object.Namespace, object.Name, err)
// This logger uses a structured logging schema in JSON format, which will / can be used further
// to access the values in the logger.

package types

import (
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	litmuschaosv1alpha1 "github.com/litmuschaos/chaos-scheduler/pkg/apis/litmuschaos/v1alpha1"
)

var (
	// Log with default name ie: controller_chaos-scheduler
	Log = logf.Log.WithName("controller_chaos-scheduler")
)

//SchedulerInfo Related information
type SchedulerInfo struct {
	Instance *litmuschaosv1alpha1.ChaosSchedule
}

var WeekDays = map[string]int{
	"sun": 0,
	"mon": 1,
	"tue": 2,
	"wed": 3,
	"thu": 4,
	"fri": 5,
	"sat": 6,
}
