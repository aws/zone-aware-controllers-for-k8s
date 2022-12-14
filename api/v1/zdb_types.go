// File generated by the kubebuilder framework:
// https://github.com/kubernetes-sigs/kubebuilder

/*
Copyright 2016 The Kubernetes Authors.
Modifications Copyright 2022 Amazon.com, Inc. or its affiliates. All Rights Reserved.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ZoneDisruptionBudgets CRD was based on the PDB resource definition:
// https://github.com/kubernetes/kubernetes/blob/05701a1309ae9f248b358bc98795605821e54b62/pkg/apis/policy/types.go#L25-L84

// ZoneDisruptionBudgetSpec defines the desired state of ZoneDisruptionBudget
type ZoneDisruptionBudgetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Selector label query over pods managed by the budget
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// Evict pod specification is allowed if at most "maxUnavailable" pods selected by
	// "selector" are unavailable in the same zone after the above operation for pod.
	// Evictions are not allowed if there are unavailable pods in other zones.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// Dryn-run mode that can be used to test the new controller before enable it
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// ZoneDisruptionBudgetStatus defines the observed state of ZoneDisruptionBudget
type ZoneDisruptionBudgetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Most recent generation observed when updating this ZDB status. DisruptionsAllowed and other
	// status information is valid only if observedGeneration equals to ZDB's object generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// DisruptedPods contains information about pods whose eviction was
	// processed by the API server eviction subresource handler but has not
	// yet been observed by the ZoneDisruptionBudget controller.
	// A pod will be in this map from the time when the API server processed the
	// eviction request to the time when the pod is seen by ZDB controller
	// as having been marked for deletion (or after a timeout). The key in the map is the name of the pod
	// and the value is the time when the API server processed the eviction request. If
	// the deletion didn't occur and a pod is still there it will be removed from
	// the list automatically by ZoneDisruptionBudget controller after some time.
	// +optional
	DisruptedPods map[string]metav1.Time `json:"disruptedPods,omitempty"`

	// Number of pod disruptions that are currently allowed *per zone*
	// +optional
	DisruptionsAllowed map[string]int32 `json:"disruptionsAllowed,omitempty"`
	// Current number of healthy pods per zone
	// +optional
	CurrentHealthy map[string]int32 `json:"currentHealthy,omitempty"`
	// Current number of unhealthy pods per zone
	// +optional
	CurrentUnhealthy map[string]int32 `json:"currentUnhealthy,omitempty"`
	// Minimum desired number of healthy pods per zone
	// +optional
	DesiredHealthy map[string]int32 `json:"desiredHealthy,omitempty"`
	// Total number of expected replicas per zone
	ExpectedPods map[string]int32 `json:"expectedPods,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=zdb

// ZoneDisruptionBudget is the Schema for the zonedisruptionbudgets API
type ZoneDisruptionBudget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneDisruptionBudgetSpec   `json:"spec,omitempty"`
	Status ZoneDisruptionBudgetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ZoneDisruptionBudgetList contains a list of ZoneDisruptionBudget
type ZoneDisruptionBudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZoneDisruptionBudget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZoneDisruptionBudget{}, &ZoneDisruptionBudgetList{})
}
