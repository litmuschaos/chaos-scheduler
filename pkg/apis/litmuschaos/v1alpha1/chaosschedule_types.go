package v1alpha1

import (
	operatorV1 "github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ChaosScheduleSpec describes a user-facing custom resource which is used by developers
// +k8s:openapi-gen=true
// to create a chaos profile
type ChaosScheduleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kube-builder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	//ChaosServiceAccount is the SA specified for chaos runner pods
	ChaosServiceAccount string `json:"chaosServiceAccount"`
	//Execution schedule of batch of chaos experiments
	Schedule Schedule `json:"schedule"`
	//ScheduleState determines whether to "halt", "abort" or "active" the schedule
	ScheduleState ScheduleState `json:"scheduleState"`
	//TODO
	//ConcurrencyPolicy will state whether two engines from the same schedule
	// can exist simultaneously or not
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`
	//EngineTemplateSpec is the spec of the engine to be created by this schedule
	EngineTemplateSpec operatorV1.ChaosEngineSpec `json:"engineTemplateSpec,omitempty"`
}

//ConcurrencyPolicy ...
type ConcurrencyPolicy string

const (
	// AllowConcurrent allows CronJobs to run concurrently.
	AllowConcurrent ConcurrencyPolicy = "Allow"

	// ForbidConcurrent forbids concurrent runs, skipping next run if previous
	// hasn't finished yet.
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
	Status ChaosStatus `json:"status"`
	//StartTime defines the starting timestamp of the schedule
	StartTime metav1.Time `json:"startTime,omitempty"`
	//EndTime defines the end timestamp of the schedule
	EndTime metav1.Time `json:"endTime,omitempty"`
	//TotalInstances defines the total no. of instances to be executed
	TotalInstances int `json:"totalInstances,omitempty"`
	//RunInstances defines number of already ran instances at that point of time
	RunInstances int `json:"runInstances,omitempty"`
	//ExpectedNextRunTime defines the approximate time at which execution of the next instance will take place
	ExpectedNextRunTime metav1.Time `json:"expectedNextRunTime,omitempty"`
}

// ChaosScheduleStatus derives information about status of individual experiments
// +k8s:openapi-gen=true
type ChaosScheduleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kube-builder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	//Status of the schedule whether active, aborted or halted
	Schedule ScheduleStatus `json:"schedule,omitempty"`
	//LastScheduleTime ...
	LastScheduleTime *metav1.Time
	//Active...
	Active []coreV1.ObjectReference
}

// Schedule defines information about schedule of chaos batch run
type Schedule struct {
	//Whether the chaos is to be scheduled at a random time or not
	Random bool `json:"random"`
	//Type of schedule should be one of ('now', 'once', 'repeat')
	Type string `json:"type"`
	//Minimum Period b/w two iterations of chaos experiments batch run
	MinChaosInterval string `json:"minChaosInterval"`
	//Number of Instances of the experiment to be run within a given time range
	InstanceCount string `json:"instanceCount"`
	//Days of week when experiments batch run is scheduled
	IncludedDays string `json:"includedDays"`
	//Start limit of the time range in which experiment is to be run
	StartTime metav1.Time `json:"startTime"`
	//End limit of the time range in which experiment is to be run
	EndTime metav1.Time `json:"endTime"`
	//Time at which experiment is to be run when Type='once'
	ExecutionTime metav1.Time `json:"executionTime"`
}

// +genclient
// +resource:path=chaosschedule
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ChaosSchedule is the Schema for the chaosschedules API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type ChaosSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosScheduleSpec   `json:"spec,omitempty"`
	Status ChaosScheduleStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ChaosScheduleList contains a list of ChaosSchedule
type ChaosScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChaosSchedule{}, &ChaosScheduleList{})
}
