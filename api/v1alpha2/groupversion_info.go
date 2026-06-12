// Package v1alpha2 contains API Schema definitions for the zitadel.truvity.io v1alpha2 API group.
// +kubebuilder:object:generate=true
// +groupName=zitadel.truvity.io
package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "zitadel.truvity.io", Version: "v1alpha2"}

	// SchemeBuilder is used to add go types to the GroupVersionResource scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&Organization{}, &OrganizationList{},
		&Project{}, &ProjectList{},
		&OIDCApp{}, &OIDCAppList{},
		&MachineUser{}, &MachineUserList{},
		&UserGrant{}, &UserGrantList{},
		&ActionTarget{}, &ActionTargetList{},
		&ActionExecution{}, &ActionExecutionList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
