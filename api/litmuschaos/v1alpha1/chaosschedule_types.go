/*
Copyright 2019 LitmusChaos Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	operatorV1 "github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChaosScheduleSpec defines the desired state of ChaosSchedule
type ChaosScheduleSpec struct {
	// ChaosServiceAccount is the SA specified for chaos runner pods
	ChaosServiceAccount string `json:"chaosServiceAccount,omitempty"`
	// Execution schedule of batch of chaos experiments
	Schedule Schedule `json:"schedule,omitempty"`
	// ScheduleState determines whether to "halt", "abort" or "active" the schedule
	ScheduleState ScheduleState `json:"scheduleState,omitempty"`
	// ConcurrencyPolicy will state whether two engines from the same schedule
	// can exist simultaneously or not
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`
	// EngineTemplateSpec is the spec of the engine to be created by this schedule
	EngineTemplateSpec operatorV1.ChaosEngineSpec `json:"engineTemplateSpec,omitempty"`
}

// ConcurrencyPolicy
type ConcurrencyPolicy string

const (
	// AllowConcurrent allows CronJobs to run concurrently.
	AllowConcurrent ConcurrencyPolicy = "Allow"
	// ForbidConcurrent forbids concurrent runs, skipping next run if previous hasn't finished yet.
	ForbidConcurrent ConcurrencyPolicy = "Forbid"
	// ReplaceConcurrent cancels currently running job and replaces it with a new one.
	ReplaceConcurrent ConcurrencyPolicy = "Replace"
)

//ScheduleState defines the current state of the schedule
type ScheduleState string

const (
	//StateActive defines that the schedule is currently active
	StateActive ScheduleState = "active"

	//StateHalted defines that the schedule is in halt and can be resumed
	StateHalted ScheduleState = "halt"

	//StateStopped defines that the schedule
	StateStopped ScheduleState = "stop"

	//StateCompleted defines that the schedule is completed
	StateCompleted ScheduleState = "complete"
)

// ChaosStatus describes current status of the schedule
type ChaosStatus string

const (
	//StatusCompleted denotes that the schedule is completed
	StatusCompleted ChaosStatus = "completed"

	//StatusRunning denotes that the schedule is running
	StatusRunning ChaosStatus = "running"

	//StatusHalted denotes that the schedule is halted
	StatusHalted ChaosStatus = "halted"

	//StatusStopped denotes the schedule is abruptly stopped in the middle of execution
	StatusStopped ChaosStatus = "stopped"
)

//ScheduleStatus describes the overall status of the schedule
type ScheduleStatus struct {
	//Status defines the current running status of the schedule
	Status ChaosStatus `json:"status,omitempty"`
	//StartTime defines the starting timestamp of the schedule
	StartTime *metav1.Time `json:"startTime,omitempty"`
	//EndTime defines the end timestamp of the schedule
	EndTime *metav1.Time `json:"endTime,omitempty"`
	//RunInstances defines number of already ran instances at that point of time
	RunInstances int `json:"runInstances,omitempty"`
	//ExpectedNextRunTime defines the approximate time at which execution of the next instance will take place
	ExpectedNextRunTime *metav1.Time `json:"expectedNextRunTime,omitempty"`
}

// ChaosScheduleStatus defines the observed state of ChaosSchedule
type ChaosScheduleStatus struct {
	// Schedule depicts status of the schedule whether active, aborted or halted
	Schedule ScheduleStatus `json:"schedule,omitempty"`
	// LastScheduleTime states the last time an engine was created
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`
	// LastScheduleCompletionTime states the last time an engine was completed
	LastScheduleCompletionTime *metav1.Time `json:"lastScheduleCompletionTime,omitempty"`
	// Active states the list of chaosengines that are currently running
	Active []coreV1.ObjectReference `json:"active,omitempty"`
}

// Schedule defines information about schedule of chaos batch run
type Schedule struct {
	// Now is for scheduling the engine immediately
	Now bool `json:"now,omitempty"`
	// Once is for scheduling the engine at a specific time
	Once *ScheduleOnce `json:"once,omitempty"`
	// Repeat is for scheduling the engine between a time range
	Repeat *ScheduleRepeat `json:"repeat,omitempty"`
}

// ScheduleOnce will contain parameters for execution the once strategy of scheduling
type ScheduleOnce struct {
	//Time at which experiment is to be run
	ExecutionTime metav1.Time `json:"executionTime"`
}

// ScheduleRepeat will contain parameters for executing the repeat strategy of scheduling
type ScheduleRepeat struct {
	TimeRange  *TimeRange               `json:"timeRange,omitempty"`
	Properties ScheduleRepeatProperties `json:"properties,omitempty"`
	WorkHours  *WorkHours               `json:"workHours,omitempty"`
	WorkDays   *WorkDays                `json:"workDays,omitempty"`
}

//TimeRange will contain time constraints for the chaos to be injected
type TimeRange struct {
	//Start limit of the time range in which experiment is to be run
	StartTime *metav1.Time `json:"startTime,omitempty"`
	//End limit of the time range in which experiment is to be run
	EndTime *metav1.Time `json:"endTime,omitempty"`
}

//ScheduleRepeatProperties will define the properties needed by the schedule to inject chaos
type ScheduleRepeatProperties struct {
	//Minimum Period b/w two iterations of chaos experiments batch run
	MinChaosInterval *MinChaosInterval `json:"minChaosInterval,omitempty"`
	//Whether the chaos is to be scheduled at a random time or not
	Random bool `json:"random,omitempty"`
}

// MinChaosInterval contains hours and minutes b/w each iterations
type MinChaosInterval struct {
	Hour   *Hour   `json:"hour,omitempty"`
	Minute *Minute `json:"minute,omitempty"`
}

// Hour contains hours and minutes b/w each schedule
type Hour struct {
	EveryNthHour    int `json:"everyNthHour,omitempty"`
	MinuteOfTheHour int `json:"minuteOfTheHour,omitempty"`
}

// Minute contains minute b/w each schedule
type Minute struct {
	EveryNthMinute int `json:"everyNthMinute,omitempty"`
}

//WorkHours specify in which hours of the day chaos is to be injected
type WorkHours struct {
	//Hours of the day when experiments batch run is scheduled
	IncludedHours string `json:"includedHours,omitempty"`
}

//WorkDays specify in which hours of the day chaos is to be injected
type WorkDays struct {
	//Days of week when experiments batch run is scheduled
	IncludedDays string `json:"includedDays,omitempty"`
}

// +genclient
// +resource:path=chaosschedule
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ChaosSchedule is the Schema for the chaosschedules API
type ChaosSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosScheduleSpec   `json:"spec,omitempty"`
	Status ChaosScheduleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ChaosScheduleList contains a list of ChaosSchedule
type ChaosScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChaosSchedule{}, &ChaosScheduleList{})
}
