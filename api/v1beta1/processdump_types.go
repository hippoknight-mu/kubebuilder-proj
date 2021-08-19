// Important: Run "make" to regenerate code after modifying this file

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProcessDumpConditionType defines condition type for ProcessDump resources
type ProcessDumpConditionType string

// These are valid conditions of processdump resource,
// they also indicate the statuses of each steps during whole dump process, "True" means steps succeed
const (
	// WorkerPodCreated indicates that workerpod has been created or not
	WorkerPodCreated ProcessDumpConditionType = "WorkerPodCreated"

	// Indicates that process list has been retrieved and shown in corresponding condition message
	ProcessListRetrieved ProcessDumpConditionType = "ProcessListRetrieved"

	// Indicates that watson dump tool has succeeded or not,
	// if succeeded, condition message should be the url to the location dump uploaded
	// if failed, condition message should give some error message
	WatsonSucceeded ProcessDumpConditionType = "WatsonSucceeded"
)

// Reason constants provide more information about ProcessDump resource conditions.
const (
	// ProcessDumpSuccess indicates that process dump file have been successfully uploaded.
	ReasonProcessDumpSuccess = "ProcessDumpSuccess"

	// DumpParameterCheckFailed indicates that controller failed to validate the dump operation parameter when user create the resource.
	ReasonDumpParameterCheckFailed = "ParameterCheckFailed"

	// NoSuchPod indicates that worker pod could not find the target pod.
	ReasonNoSuchPod = "NoSuchPod"

	// DumpOperationTimeout indicates that dump operation is time out
	ReasonDumpOperationTimeout = "DumpOperationTimeout"

	// DumpOperationTimeout indicates that dump operation have some error in dump operation
	ReasonDumpOperationExecuteError = "ProcessDumpExecuteError"
)

// ProcDumpStatus defines the observed state of ProcDump
type ProcessDumpStatus struct {
	// StartTime defines start date and time when dump operation start.
	// +optional
	StartTime *metav1.Time `json:"startTime"`

	// EndTime defines start date and time when dump operation end.
	// +optional
	EndTime *metav1.Time `json:"endTime"`

	// LastUpdateTime defines date and time when DumpStatus was successfully changed.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime"`

	// Conditions define a set of conditions that process dump controller observed on this custom resource
	// +optional
	Conditions []ProcessDumpCondition `json:"conditions"`

	// WorkerPod belongs to this resource
	// +optional
	WorkerPodName string `json:"workerPodName"`
}

type ProcessDumpCondition struct {
	// Type specifies condition type for ProcessDump.
	// See defined constants of this type above for a list of valid condition types.
	Type ProcessDumpConditionType `json:"type"`

	// Status specifies observed condition status for specific condition type. Valid values are "True", "False", "Unknown".
	// The absense of condition is equal to "Unknown" status.
	// For this controller "Unknown" status corresponds to condition not being observed.
	Status metav1.ConditionStatus `json:"status"`

	// Reason is one-word CamelCase reson for the condition latest transition.
	Reason string `json:"reason"`

	// Message is human-readable message explaining details about latest transition.
	Message string `json:"message"`

	// LastTransitionTime is time when condition was created or last updated.
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
}

// ProcessDumpSpec is the spec for a ProcessDump resource
type ProcessDumpSpec struct {
	PodName       string `json:"podName"`
	ProcessName   string `json:"processName"`
	ProcessID     int    `json:"processID"`
	ContainerName string `json:"containerName"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProcessDump is the Schema for the processdumps API
type ProcessDump struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProcessDumpSpec   `json:"spec,omitempty"`
	Status ProcessDumpStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProcessDumpList contains a list of ProcessDump
type ProcessDumpList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProcessDump `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProcessDump{}, &ProcessDumpList{})
}
