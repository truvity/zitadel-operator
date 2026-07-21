package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MachineUserSpec defines the desired state of MachineUser.
type MachineUserSpec struct {
	// Instance optionally pins this resource to one operator's instance
	// identity — the operator config's instanceAlias, defaulting to its
	// domain (v0.18 dual-serving). When set to another identity, the CR is
	// ignored entirely so the owning operator can manage it. When empty
	// while the namespace is served by two operators, both fail closed with
	// an AmbiguousInstance condition.
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

	// UserName is the login name for the machine user.
	UserName string `json:"userName"`

	// Name is the display name of the machine user.
	// If empty, the Kubernetes resource name is used.
	// +optional
	Name string `json:"name,omitempty"`

	// Description is an optional description of the machine user.
	// +optional
	Description string `json:"description,omitempty"`

	// AccessTokenType specifies the access token format for this machine user.
	// +kubebuilder:validation:Enum=bearer;jwt
	// +optional
	AccessTokenType string `json:"accessTokenType,omitempty"`

	// Roles are project role grants for this machine user (v0.18, INF-426).
	// The grant target is, in order of precedence: the project named by
	// ProjectRef/ProjectId (v0.19), the namespace's resolved scope project
	// (scope maps), or a previously recorded status.projectId. Grants never
	// widen beyond the resolved scope when scope maps are active.
	// +optional
	Roles []string `json:"roles,omitempty"`

	// ProjectRef references a Project CR whose project the role grant is
	// created on (v0.19 fleet shape: declare the project in-namespace and
	// grant roles on it without scope maps). Only used with Roles.
	// Mutually exclusive with ProjectId.
	// +optional
	ProjectRef *ResourceRef `json:"projectRef,omitempty"`

	// ProjectId references a pre-existing Zitadel project by raw ID as the
	// role grant target. Only used with Roles.
	// Mutually exclusive with ProjectRef.
	// +optional
	ProjectId string `json:"projectId,omitempty"`

	// Key configures machine key lifecycle (v0.18, INF-426).
	// +optional
	Key *MachineKeySpec `json:"key,omitempty"`

	// KeySecretRef references the Secret where the generated key JSON will be stored.
	KeySecretRef MachineKeySecretRef `json:"keySecretRef"`
}

// MachineKeySpec configures machine key rotation.
type MachineKeySpec struct {
	// RotateAfter enables dual-key rotation: once the current key is older
	// than this duration, a new key is minted and swapped into the Secret;
	// the old key is revoked after RotationGrace (two keys coexist during
	// the overlap). Example: "2160h" (90 days). Empty = never rotate
	// (pre-v0.18 behavior).
	// +optional
	RotateAfter *metav1.Duration `json:"rotateAfter,omitempty"`

	// RotationGrace is how long the old key stays valid after rotation.
	// Defaults to 5m.
	// +optional
	RotationGrace *metav1.Duration `json:"rotationGrace,omitempty"`
}

// MachineUserStatus defines the observed state of MachineUser.
type MachineUserStatus struct {
	// UserId is the Zitadel user ID.
	UserId string `json:"userId,omitempty"`

	// OrganizationId is the resolved organization ID.
	OrganizationId string `json:"organizationId,omitempty"`

	// ProjectId is the scope project the roles are granted on (v0.18).
	ProjectId string `json:"projectId,omitempty"`

	// GrantId is the Zitadel user grant carrying spec.roles (v0.18).
	GrantId string `json:"grantId,omitempty"`

	// KeyId is the current machine key (v0.18 rotation bookkeeping).
	KeyId string `json:"keyId,omitempty"`

	// KeyCreatedAt is when the current key was minted.
	KeyCreatedAt *metav1.Time `json:"keyCreatedAt,omitempty"`

	// PreviousKeyId is the rotated-out key awaiting revocation.
	PreviousKeyId string `json:"previousKeyId,omitempty"`

	// PreviousKeyRevokeAt is when the rotated-out key gets revoked.
	PreviousKeyRevokeAt *metav1.Time `json:"previousKeyRevokeAt,omitempty"`

	// Ready indicates whether the MachineUser is successfully synced.
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
// +kubebuilder:printcolumn:name="UserID",type=string,JSONPath=`.status.userId`
// +kubebuilder:printcolumn:name="UserName",type=string,JSONPath=`.spec.userName`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// MachineUser is the Schema for the machineusers API.
type MachineUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineUserSpec   `json:"spec,omitempty"`
	Status MachineUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MachineUserList contains a list of MachineUser.
type MachineUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineUser `json:"items"`
}

// DisplayName returns the machine user display name.
// Falls back to the Kubernetes resource name if spec.name is empty.
func (m *MachineUser) DisplayName() string {
	if m.Spec.Name != "" {
		return m.Spec.Name
	}
	return m.Name
}
