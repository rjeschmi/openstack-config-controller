// +groupName=openstack.ayr.ca

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// EDIT THIS FILE: simple CRD types for OpenStack block storage listing

// GroupVersion definitions
var (
	GroupVersion  = schema.GroupVersion{Group: "openstack.ayr.ca", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
)

// OpenStackBlockStorageSpec defines the desired state
type OpenStackBlockStorageSpec struct {
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

func (in *OpenStackBlockStorage) DeepCopyInto(out *OpenStackBlockStorage) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Status.Volumes != nil {
		out.Status.Volumes = make([]VolumeStatus, len(in.Status.Volumes))
		copy(out.Status.Volumes, in.Status.Volumes)
	} else {
		out.Status.Volumes = nil
	}
}

func (in *OpenStackBlockStorage) DeepCopy() *OpenStackBlockStorage {
	if in == nil {
		return nil
	}
	out := new(OpenStackBlockStorage)
	in.DeepCopyInto(out)
	return out
}

func (in *OpenStackBlockStorage) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *OpenStackBlockStorageList) DeepCopyInto(out *OpenStackBlockStorageList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]OpenStackBlockStorage, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	} else {
		out.Items = nil
	}
}

func (in *OpenStackBlockStorageList) DeepCopy() *OpenStackBlockStorageList {
	if in == nil {
		return nil
	}
	out := new(OpenStackBlockStorageList)
	in.DeepCopyInto(out)
	return out
}

func (in *OpenStackBlockStorageList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&OpenStackBlockStorage{}, &OpenStackBlockStorageList{})
}
