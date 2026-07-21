package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectSpec defines the desired state of Project.
type ProjectSpec struct {
	// Instance optionally pins this resource to a specific Zitadel instance
	// domain (v0.18 dual-serving). When set to a domain other than this
	// operator's binding, the CR is ignored entirely so the owning operator
	// can manage it. When empty while the namespace is served by two
	// operators, both fail closed with an AmbiguousInstance condition.
	// +optional
	Instance string `json:"instance,omitempty"`

	// OrganizationRef references an Organization CR managed by this operator.
	// Mutually exclusive with OrganizationId.
	// +optional
	OrganizationRef *ResourceRef `json:"organizationRef,omitempty"`

	// OrganizationId references a pre-existing Zitadel organization by raw ID.
	// Mutually exclusive with OrganizationRef.
	// +optional
	OrganizationId string `json:"organizationId,omitempty"`

	// Name is the display name of the project in Zitadel.
	// If empty, the Kubernetes resource name is used.
	// +optional
	Name string `json:"name,omitempty"`

	// AssertRolesOnAuth determines whether roles are asserted on authentication.
	// +optional
	AssertRolesOnAuth bool `json:"assertRolesOnAuth,omitempty"`

	// CheckAuthorizationOnAuth enables authorization check on authentication.
	// When true, only users with explicit role assignments can authenticate,
	// and user grants are loaded into the Action context (ctx.v1.user.grants).
	// +optional
	CheckAuthorizationOnAuth bool `json:"checkAuthorizationOnAuth,omitempty"`

	// Roles is the list of roles defined for this project.
	// +optional
	Roles []string `json:"roles,omitempty"`
}

// ProjectStatus defines the observed state of Project.
type ProjectStatus struct {
	// ProjectId is the Zitadel project ID.
	ProjectId string `json:"projectId,omitempty"`

	// OrganizationId is the resolved organization ID this project belongs to.
	OrganizationId string `json:"organizationId,omitempty"`

	// Ready indicates whether the Project is successfully synced.
	Ready bool `json:"ready,omitempty"`

	// LastSyncTime is the last time the resource was synced with Zitadel.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ProjectID",type=string,JSONPath=`.status.projectId`
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

// DisplayName returns the project display name for Zitadel.
// Falls back to the Kubernetes resource name if spec.name is empty.
func (p *Project) DisplayName() string {
	if p.Spec.Name != "" {
		return p.Spec.Name
	}
	return p.Name
}
