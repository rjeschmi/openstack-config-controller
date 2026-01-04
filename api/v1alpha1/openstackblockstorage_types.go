// +groupName=openstack.ayr.ca

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// EDIT THIS FILE: simple CRD types for OpenStack block storage listing

// GroupVersion definitions
var (
	GroupVersion  = schema.GroupVersion{Group: "openstack.ayr.ca", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
)

// +kubebuilder:object:generate:=true
// OpenStackBlockStorageSpec defines the desired state
type OpenStackBlockStorageSpec struct {
	// Cloud is the name of the cloud in clouds.yaml to use for authentication.
	Cloud string `json:"cloud"`
	// CloudConfig is a reference to the secret containing the clouds.yaml file.
	CloudConfig corev1.SecretKeySelector `json:"cloudConfig"`
	// ProjectName is an optional OpenStack project to query
	ProjectName string `json:"projectName,omitempty"`
}

// VolumeStatus describes a single volume from OpenStack
type VolumeStatus struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	SizeGB int    `json:"sizeGB,omitempty"`
	Status string `json:"status,omitempty"`
}

// +kubebuilder:object:generate:=true
// OpenStackBlockStorageStatus defines the observed state
type OpenStackBlockStorageStatus struct {
	// Volumes lists block storage volumes discovered in OpenStack
	Volumes []VolumeStatus `json:"volumes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// OpenStackBlockStorage is the Schema for the openstackblockstorages API
type OpenStackBlockStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenStackBlockStorageSpec   `json:"spec,omitempty"`
	Status OpenStackBlockStorageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// OpenStackBlockStorageList contains a list of OpenStackBlockStorage
type OpenStackBlockStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenStackBlockStorage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenStackBlockStorage{}, &OpenStackBlockStorageList{})
}
