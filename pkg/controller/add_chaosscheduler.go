package controller

import (
	"github.com/litmuschaos/chaos-scheduler/pkg/controller/chaosscheduler"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, chaosscheduler.Add)
}
