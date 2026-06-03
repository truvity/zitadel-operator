package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectSpec defines the desired state of Project.
type ProjectSpec struct {
	// OrganizationId is the Zitadel organization ID this project belongs to.
	// If empty, the default organization of the instance is used.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// AssertRolesOnAuth determines whether roles are asserted on authentication.
	AssertRolesOnAuth bool `json:"assertRolesOnAuth,omitempty"`

	// CheckAuthorizationOnAuth enables authorization check on authentication.
	// When true, only users with explicit role assignments can authenticate,
	// and user grants are loaded into the Action context (ctx.v1.user.grants).
	// Required for the groups claim Action to work.
	// +optional
	CheckAuthorizationOnAuth bool `json:"checkAuthorizationOnAuth,omitempty"`

	// Roles is the list of roles defined for this project.
	Roles []string `json:"roles,omitempty"`
}

// ProjectStatus defines the observed state of Project.
type ProjectStatus struct {
	// ProjectId is the Zitadel project ID.
	ProjectId string `json:"projectId,omitempty"`

	// Ready indicates whether the Project is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// Project is the Schema for the projects API.
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec,omitempty"`
	Status ProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}
