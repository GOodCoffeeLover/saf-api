/*
Copyright 2025 GoodCoffeeLover.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SAFMachineSpec defines the desired state of SAFMachine
type SAFMachineSpec struct {
	// +optional
	ConnectionConfig *ConnectionConfig `json:"connectionConfig,omitempty"`
}

type ConnectionConfig struct {
	Address  string `json:"address"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// SAFMachineStatus defines the observed state of SAFMachine.
type SAFMachineStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SAFMachine is the Schema for the safmachines API
type SAFMachine struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of SAFMachine
	// +required
	Spec SAFMachineSpec `json:"spec"`

	// status defines the observed state of SAFMachine
	// +optional
	Status SAFMachineStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// SAFMachineList contains a list of SAFMachine
type SAFMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SAFMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SAFMachine{}, &SAFMachineList{})
}
