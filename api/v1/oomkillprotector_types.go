/*


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
)

// +kubebuilder:validation:Enum=Init;UnderProtected;
type ProtectorStatus string

const (
	Init           ProtectorStatus = "Init"
	UnderProtected ProtectorStatus = "UnderProtected"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OOMKillProtectorSpec defines the desired state of OOMKillProtector
type OOMKillProtectorSpec struct {
	Selector map[string]string `json:"selector,omitempty"`

	ContainerName string `json:"containerName,omitempty"`

	// +kubebuilder:validation:Minimum=70
	// +kubebuilder:validation:Maximum=100
	ThresholdRatio int32 `json:"thresholdRatio,omitempty"`

	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=300
	// +kubebuilder:validation:Maximum=300
	ScalaRatio int32 `json:"scalaRatio,omitempty"`
}

// OOMKillProtectorStatus defines the observed state of OOMKillProtector
type OOMKillProtectorStatus struct {
	// +optional
	ProtectorStatus *ProtectorStatus `json:"protectStatus,omitempty"`
	// +optional
	StatusUpdateTime *metav1.Time `json:"statusUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true

// OOMKillProtector is the Schema for the oomkillprotectors API
type OOMKillProtector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OOMKillProtectorSpec   `json:"spec,omitempty"`
	Status OOMKillProtectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OOMKillProtectorList contains a list of OOMKillProtector
type OOMKillProtectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OOMKillProtector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OOMKillProtector{}, &OOMKillProtectorList{})
}
