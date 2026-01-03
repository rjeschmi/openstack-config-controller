/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUTHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenStackVolumeSpec defines the desired state of OpenStackVolume
type OpenStackVolumeSpec struct {
	// ID is the UUID of the volume.
	ID string `json:"id"`
	// Name is the name of the volume.
	Name string `json:"name"`
	// Status is the status of the volume.
	Status string `json:"status"`
	// Size is the size of the volume in GB.
	Size int `json:"size"`
	// AvailabilityZone is the availability zone of the volume.
	AvailabilityZone string `json:"availability_zone"`
	// CreatedAt is the date and time the volume was created.
	CreatedAt metav1.Time `json:"created_at"`
	// VolumeType is the type of the volume.
	VolumeType string `json:"volume_type"`
	// Bootable is whether the volume is bootable.
	Bootable string `json:"bootable"`
	// Encrypted is whether the volume is encrypted.
	Encrypted bool `json:"encrypted"`
}

// OpenStackVolumeStatus defines the observed state of OpenStackVolume
type OpenStackVolumeStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=openstackvolumes,scope=Namespaced,shortName=osv

// OpenStackVolume is the Schema for the openstackvolumes API
type OpenStackVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenStackVolumeSpec   `json:"spec,omitempty"`
	Status OpenStackVolumeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OpenStackVolumeList contains a list of OpenStackVolume
type OpenStackVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenStackVolume `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenStackVolume{}, &OpenStackVolumeList{})
}
