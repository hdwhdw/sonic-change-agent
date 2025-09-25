package apis

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkDevice represents a SONiC network device
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NetworkDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkDeviceSpec   `json:"spec,omitempty"`
	Status NetworkDeviceStatus `json:"status,omitempty"`
}

// NetworkDeviceList contains a list of NetworkDevice
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NetworkDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkDevice `json:"items"`
}

// NetworkDeviceSpec defines the desired state of NetworkDevice
type NetworkDeviceSpec struct {
	OS      OSSpec      `json:"os,omitempty"`
	Preload PreloadSpec `json:"preload,omitempty"`
}

// OSSpec defines the OS configuration
type OSSpec struct {
	OSType         string `json:"osType,omitempty"`
	DesiredVersion string `json:"desiredVersion,omitempty"`
}

// PreloadSpec defines the preload operation
type PreloadSpec struct {
	TargetVersion string            `json:"targetVersion,omitempty"`
	ImageURL      string            `json:"imageURL,omitempty"`
	Checksum      ChecksumSpec      `json:"checksum,omitempty"`
	RequestID     string            `json:"requestId,omitempty"`
	Mode          string            `json:"mode,omitempty"`
}

// ChecksumSpec defines checksum validation
type ChecksumSpec struct {
	MD5 string `json:"md5,omitempty"`
}

// NetworkDeviceStatus defines the observed state of NetworkDevice
type NetworkDeviceStatus struct {
	State      string            `json:"state,omitempty"`
	OS         OSStatus          `json:"os,omitempty"`
	Preload    PreloadStatus     `json:"preload,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// OSStatus defines the current OS state
type OSStatus struct {
	CurrentVersion   string `json:"currentVersion,omitempty"`
	PreloadedVersion string `json:"preloadedVersion,omitempty"`
}

// PreloadStatus defines the current preload operation state
type PreloadStatus struct {
	ObservedRequestID string `json:"observedRequestId,omitempty"`
	Phase             string `json:"phase,omitempty"`
	Progress          int    `json:"progress,omitempty"`
	Message           string `json:"message,omitempty"`
}