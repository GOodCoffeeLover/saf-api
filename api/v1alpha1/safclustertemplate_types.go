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
	capiv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// SAFClusterTemplateSpec defines the desired state of SAFClusterTemplate
type SAFClusterTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of SAFClusterTemplate. Edit safclustertemplate_types.go to remove/update
	// +optional
	Template SAFClusterTemplateResource `json:"template"`
}

type SAFClusterTemplateResource struct {
	ObjectMeta capiv1beta2.ObjectMeta `json:"metadata,omitempty,omitzero"`
	Spec       SAFClusterSpec         `json:"spec"`
}

// +kubebuilder:object:root=true

// SAFClusterTemplate is the Schema for the safclustertemplates API
type SAFClusterTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of SAFClusterTemplate
	// +required
	Spec SAFClusterTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// SAFClusterTemplateList contains a list of SAFClusterTemplate
type SAFClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SAFClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SAFClusterTemplate{}, &SAFClusterTemplateList{})
}
