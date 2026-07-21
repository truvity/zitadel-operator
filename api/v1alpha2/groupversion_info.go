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
		&APIApp{}, &APIAppList{},
		&SAMLApp{}, &SAMLAppList{},
		&ApplicationKey{}, &ApplicationKeyList{},
		&PersonalAccessToken{}, &PersonalAccessTokenList{},
		&MachineUser{}, &MachineUserList{},
		&UserGrant{}, &UserGrantList{},
		&ActionTarget{}, &ActionTargetList{},
		&ActionExecution{}, &ActionExecutionList{},
		&ProjectMember{}, &ProjectMemberList{},
		&ProjectGrantMember{}, &ProjectGrantMemberList{},
		&OrgMetadata{}, &OrgMetadataList{},
		&Domain{}, &DomainList{},
		&ProjectGrant{}, &ProjectGrantList{},
		&IdentityProvider{}, &IdentityProviderList{},
		&DefaultLoginPolicy{}, &DefaultLoginPolicyList{},
		&DefaultDomainPolicy{}, &DefaultDomainPolicyList{},
		&GoogleIdP{}, &GoogleIdPList{},
		&LoginPolicy{}, &LoginPolicyList{},
		&PasswordComplexityPolicy{}, &PasswordComplexityPolicyList{},
		&LockoutPolicy{}, &LockoutPolicyList{},
		&EmailProvider{}, &EmailProviderList{},
		&HumanUser{}, &HumanUserList{},
		&OrgMember{}, &OrgMemberList{},
		&InstanceMember{}, &InstanceMemberList{},
		&LabelPolicy{}, &LabelPolicyList{},
		&NotificationPolicy{}, &NotificationPolicyList{},
		&PasswordAgePolicy{}, &PasswordAgePolicyList{},
		&SmsProvider{}, &SmsProviderList{},
		&GitHubIdP{}, &GitHubIdPList{},
		&DefaultLockoutPolicy{}, &DefaultLockoutPolicyList{},
		&DefaultPasswordComplexityPolicy{}, &DefaultPasswordComplexityPolicyList{},
		&DefaultPasswordAgePolicy{}, &DefaultPasswordAgePolicyList{},
		&DefaultNotificationPolicy{}, &DefaultNotificationPolicyList{},
		&DefaultLabelPolicy{}, &DefaultLabelPolicyList{},
		&DefaultPrivacyPolicy{}, &DefaultPrivacyPolicyList{},
		&DefaultOIDCSettings{}, &DefaultOIDCSettingsList{},
		&PrivacyPolicy{}, &PrivacyPolicyList{},
		&DefaultMessageText{}, &DefaultMessageTextList{},
		&MessageText{}, &MessageTextList{},
		&ScopeMap{}, &ScopeMapList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
