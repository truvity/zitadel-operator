# API Reference

## Packages
- [zitadel.truvity.io/v1alpha2](#zitadeltruvityiov1alpha2)


## zitadel.truvity.io/v1alpha2

Package v1alpha2 contains API Schema definitions for the zitadel.truvity.io v1alpha2 API group.

### Resource Types
- [APIApp](#apiapp)
- [ActionExecution](#actionexecution)
- [ActionTarget](#actiontarget)
- [ApplicationKey](#applicationkey)
- [DefaultDomainPolicy](#defaultdomainpolicy)
- [DefaultLabelPolicy](#defaultlabelpolicy)
- [DefaultLockoutPolicy](#defaultlockoutpolicy)
- [DefaultLoginPolicy](#defaultloginpolicy)
- [DefaultMessageText](#defaultmessagetext)
- [DefaultNotificationPolicy](#defaultnotificationpolicy)
- [DefaultOIDCSettings](#defaultoidcsettings)
- [DefaultPasswordAgePolicy](#defaultpasswordagepolicy)
- [DefaultPasswordComplexityPolicy](#defaultpasswordcomplexitypolicy)
- [DefaultPrivacyPolicy](#defaultprivacypolicy)
- [Domain](#domain)
- [EmailProvider](#emailprovider)
- [GitHubIdP](#githubidp)
- [GoogleIdP](#googleidp)
- [HumanUser](#humanuser)
- [IdentityProvider](#identityprovider)
- [InstanceMember](#instancemember)
- [LabelPolicy](#labelpolicy)
- [LockoutPolicy](#lockoutpolicy)
- [LoginPolicy](#loginpolicy)
- [MachineUser](#machineuser)
- [MessageText](#messagetext)
- [NotificationPolicy](#notificationpolicy)
- [OIDCApp](#oidcapp)
- [OrgMember](#orgmember)
- [OrgMetadata](#orgmetadata)
- [Organization](#organization)
- [PasswordAgePolicy](#passwordagepolicy)
- [PasswordComplexityPolicy](#passwordcomplexitypolicy)
- [PersonalAccessToken](#personalaccesstoken)
- [PrivacyPolicy](#privacypolicy)
- [Project](#project)
- [ProjectGrant](#projectgrant)
- [ProjectGrantMember](#projectgrantmember)
- [ProjectMember](#projectmember)
- [ProjectRole](#projectrole)
- [SAMLApp](#samlapp)
- [ScopeMap](#scopemap)
- [SmsProvider](#smsprovider)
- [UserGrant](#usergrant)



#### APIApp



APIApp is the Schema for the apiapps API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `APIApp` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[APIAppSpec](#apiappspec)_ |  |  |  |
| `status` _[APIAppStatus](#apiappstatus)_ |  |  |  |


#### APIAppSpec



APIAppSpec defines the desired state of APIApp.



_Appears in:_
- [APIApp](#apiapp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `name` _string_ | Name is the display name of the application in Zitadel.<br />If empty, the Kubernetes resource name is used. |  | Optional: \{\} <br /> |
| `authMethod` _string_ | AuthMethod is the API authentication method. |  | Enum: [basic private_key_jwt] <br /> |
| `secretRef` _[SecretRefSpec](#secretrefspec)_ | SecretRef references the Secret where the client credentials will be stored. |  |  |


#### APIAppStatus



APIAppStatus defines the observed state of APIApp.



_Appears in:_
- [APIApp](#apiapp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applicationId` _string_ | ApplicationId is the Zitadel application ID. |  |  |
| `clientId` _string_ | ClientId is the API client ID assigned by Zitadel. |  |  |
| `projectId` _string_ | ProjectId is the resolved project ID this app belongs to. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID (inherited from project). |  |  |
| `ready` _boolean_ | Ready indicates whether the APIApp is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ActionCondition



ActionCondition defines the trigger condition for an execution.



_Appears in:_
- [ActionExecutionSpec](#actionexecutionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `function` _string_ | Function is a Zitadel function condition (e.g., "/zitadel.session.v2.SessionService/SetSession").<br />Mutually exclusive with Request and Event. |  | Optional: \{\} <br /> |
| `request` _string_ | Request is a Zitadel request condition (e.g., "/zitadel.user.v2.UserService/AddHumanUser").<br />Mutually exclusive with Function and Event. |  | Optional: \{\} <br /> |
| `event` _string_ | Event is a Zitadel event condition (e.g., "user.human.added").<br />Mutually exclusive with Function and Request. |  | Optional: \{\} <br /> |


#### ActionExecution



ActionExecution is the Schema for the actionexecutions API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ActionExecution` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ActionExecutionSpec](#actionexecutionspec)_ |  |  |  |
| `status` _[ActionExecutionStatus](#actionexecutionstatus)_ |  |  |  |


#### ActionExecutionSpec



ActionExecutionSpec defines the desired state of ActionExecution.



_Appears in:_
- [ActionExecution](#actionexecution)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `condition` _[ActionCondition](#actioncondition)_ | Condition defines when the execution triggers. |  |  |
| `targets` _[ActionExecutionTarget](#actionexecutiontarget) array_ | Targets is the list of target references to invoke. |  |  |


#### ActionExecutionStatus



ActionExecutionStatus defines the observed state of ActionExecution.



_Appears in:_
- [ActionExecution](#actionexecution)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the ActionExecution is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ActionExecutionTarget



ActionExecutionTarget references a target to invoke in the execution.



_Appears in:_
- [ActionExecutionSpec](#actionexecutionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targetRef` _[ResourceRef](#resourceref)_ | TargetRef references an ActionTarget CR managed by this operator.<br />Mutually exclusive with TargetId. |  | Optional: \{\} <br /> |
| `targetId` _string_ | TargetId references a pre-existing Zitadel target by raw ID.<br />Mutually exclusive with TargetRef. |  | Optional: \{\} <br /> |


#### ActionTarget



ActionTarget is the Schema for the actiontargets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ActionTarget` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ActionTargetSpec](#actiontargetspec)_ |  |  |  |
| `status` _[ActionTargetStatus](#actiontargetstatus)_ |  |  |  |


#### ActionTargetPayloadType

_Underlying type:_ _string_

ActionTargetPayloadType defines how the payload is formatted and secured.

_Validation:_
- Enum: [json jwt jwe]

_Appears in:_
- [ActionTargetSpec](#actiontargetspec)

| Field | Description |
| --- | --- |
| `json` | ActionTargetPayloadTypeJSON sends the payload as JSON with X-ZITADEL-Signature header.<br /> |
| `jwt` | ActionTargetPayloadTypeJWT sends the payload as a signed JWT.<br />The receiver can verify authenticity and integrity using the signing key.<br /> |
| `jwe` | ActionTargetPayloadTypeJWE sends the payload as an encrypted JWT.<br /> |


#### ActionTargetSpec



ActionTargetSpec defines the desired state of ActionTarget.



_Appears in:_
- [ActionTarget](#actiontarget)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the display name of the target in Zitadel.<br />If empty, the Kubernetes resource name is used. |  | Optional: \{\} <br /> |
| `endpoint` _string_ | Endpoint is the HTTP(S) URL the target will call. |  |  |
| `timeout` _string_ | Timeout is the request timeout duration (e.g., "10s", "30s"). |  | Optional: \{\} <br /> |
| `interruptOnError` _boolean_ | InterruptOnError determines whether the action flow stops if this target fails. |  | Optional: \{\} <br /> |
| `targetType` _[ActionTargetType](#actiontargettype)_ | TargetType defines the type of target and how Zitadel treats the response.<br />- restCall: reads the response body (required for append_claims)<br />- restWebhook: only checks status code, ignores body<br />- restAsync: fire-and-forget, does not wait for response<br />Default: restCall | restCall | Enum: [restCall restWebhook restAsync] <br />Optional: \{\} <br /> |
| `payloadType` _[ActionTargetPayloadType](#actiontargetpayloadtype)_ | PayloadType defines how the payload is formatted and secured.<br />- json: JSON body with X-ZITADEL-Signature header (default)<br />- jwt: signed JWT body (receiver verifies via JWKS)<br />- jwe: encrypted JWT body<br />Default: json | json | Enum: [json jwt jwe] <br />Optional: \{\} <br /> |


#### ActionTargetStatus



ActionTargetStatus defines the observed state of ActionTarget.



_Appears in:_
- [ActionTarget](#actiontarget)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targetId` _string_ | TargetId is the Zitadel target ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the ActionTarget is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ActionTargetType

_Underlying type:_ _string_

ActionTargetType defines the type of target and how the response is treated.

_Validation:_
- Enum: [restCall restWebhook restAsync]

_Appears in:_
- [ActionTargetSpec](#actiontargetspec)

| Field | Description |
| --- | --- |
| `restCall` | ActionTargetTypeRestCall makes a POST request and reads the response body.<br />This is required when Zitadel needs to read the response (e.g., append_claims).<br /> |
| `restWebhook` | ActionTargetTypeRestWebhook makes a POST request but ignores the response body.<br />Only the status code is checked.<br /> |
| `restAsync` | ActionTargetTypeRestAsync makes an asynchronous POST request.<br />Zitadel does not wait for the response. Typically used for event executions.<br /> |


#### ApplicationKey



ApplicationKey is the Schema for the applicationkeys API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ApplicationKey` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ApplicationKeySpec](#applicationkeyspec)_ |  |  |  |
| `status` _[ApplicationKeyStatus](#applicationkeystatus)_ |  |  |  |


#### ApplicationKeySpec



ApplicationKeySpec defines the desired state of ApplicationKey.



_Appears in:_
- [ApplicationKey](#applicationkey)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `appRef` _[ResourceRef](#resourceref)_ | AppRef references an OIDCApp or APIApp CR managed by this operator.<br />Mutually exclusive with AppId. |  | Optional: \{\} <br /> |
| `appId` _string_ | AppId references a pre-existing Zitadel application by raw ID.<br />Mutually exclusive with AppRef. |  | Optional: \{\} <br /> |
| `keyType` _string_ | KeyType specifies the key format. Currently only JSON is supported. | json | Enum: [json] <br />Optional: \{\} <br /> |
| `expirationDate` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | ExpirationDate is the optional expiration timestamp for the key.<br />If not set, a 10-year expiration is used. |  | Optional: \{\} <br /> |
| `keySecretRef` _[MachineKeySecretRef](#machinekeysecretref)_ | KeySecretRef references the Secret where the key JSON will be stored. |  |  |


#### ApplicationKeyStatus



ApplicationKeyStatus defines the observed state of ApplicationKey.



_Appears in:_
- [ApplicationKey](#applicationkey)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `keyId` _string_ | KeyId is the Zitadel application key ID. |  |  |
| `projectId` _string_ | ProjectId is the resolved project ID. |  |  |
| `appId` _string_ | AppId is the resolved application ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the ApplicationKey is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultDomainPolicy



DefaultDomainPolicy is the Schema for the defaultdomainpolicies API.
It manages the instance-level default domain policy (IAM_OWNER, Admin API).
Singleton per instance: reconcile reads the current default, diffs all fields, updates on drift.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultDomainPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultDomainPolicySpec](#defaultdomainpolicyspec)_ |  |  |  |
| `status` _[DefaultDomainPolicyStatus](#defaultdomainpolicystatus)_ |  |  |  |


#### DefaultDomainPolicySpec



DefaultDomainPolicySpec defines the desired state of DefaultDomainPolicy.



_Appears in:_
- [DefaultDomainPolicy](#defaultdomainpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userLoginMustBeDomain` _boolean_ | UserLoginMustBeDomain requires user login names to include the org domain. |  | Optional: \{\} <br /> |
| `validateOrgDomains` _boolean_ | ValidateOrgDomains requires organization domains to be verified via DNS. |  | Optional: \{\} <br /> |
| `smtpSenderAddressMatchesInstanceDomain` _boolean_ | SmtpSenderAddressMatchesInstanceDomain requires SMTP sender to match the instance domain. |  | Optional: \{\} <br /> |


#### DefaultDomainPolicyStatus



DefaultDomainPolicyStatus defines the observed state of DefaultDomainPolicy.



_Appears in:_
- [DefaultDomainPolicy](#defaultdomainpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultDomainPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultLabelPolicy



DefaultLabelPolicy is the Schema for the defaultlabelpolicies API.
It manages the instance-level default label/branding policy (IAM_OWNER, Admin API). Singleton.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultLabelPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultLabelPolicySpec](#defaultlabelpolicyspec)_ |  |  |  |
| `status` _[DefaultLabelPolicyStatus](#defaultlabelpolicystatus)_ |  |  |  |


#### DefaultLabelPolicySpec



DefaultLabelPolicySpec defines the desired state of DefaultLabelPolicy.



_Appears in:_
- [DefaultLabelPolicy](#defaultlabelpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `primaryColor` _string_ | PrimaryColor is the primary brand color (hex, e.g. "#5469d4"). |  | Optional: \{\} <br /> |
| `backgroundColor` _string_ | BackgroundColor is the background color (hex). |  | Optional: \{\} <br /> |
| `warnColor` _string_ | WarnColor is the warning color (hex). |  | Optional: \{\} <br /> |
| `fontColor` _string_ | FontColor is the font color (hex). |  | Optional: \{\} <br /> |
| `primaryColorDark` _string_ | PrimaryColorDark is the primary color for dark mode (hex). |  | Optional: \{\} <br /> |
| `backgroundColorDark` _string_ | BackgroundColorDark is the background color for dark mode (hex). |  | Optional: \{\} <br /> |
| `warnColorDark` _string_ | WarnColorDark is the warning color for dark mode (hex). |  | Optional: \{\} <br /> |
| `fontColorDark` _string_ | FontColorDark is the font color for dark mode (hex). |  | Optional: \{\} <br /> |
| `hideLoginNameSuffix` _boolean_ | HideLoginNameSuffix hides the @domain suffix on the login screen. |  | Optional: \{\} <br /> |
| `disableWatermark` _boolean_ | DisableWatermark disables the Zitadel watermark on login pages. |  | Optional: \{\} <br /> |


#### DefaultLabelPolicyStatus



DefaultLabelPolicyStatus defines the observed state of DefaultLabelPolicy.



_Appears in:_
- [DefaultLabelPolicy](#defaultlabelpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultLabelPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultLockoutPolicy



DefaultLockoutPolicy is the Schema for the defaultlockoutpolicies API.
It manages the instance-level default lockout policy (IAM_OWNER, Admin API). Singleton.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultLockoutPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultLockoutPolicySpec](#defaultlockoutpolicyspec)_ |  |  |  |
| `status` _[DefaultLockoutPolicyStatus](#defaultlockoutpolicystatus)_ |  |  |  |


#### DefaultLockoutPolicySpec



DefaultLockoutPolicySpec defines the desired state of DefaultLockoutPolicy.



_Appears in:_
- [DefaultLockoutPolicy](#defaultlockoutpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxPasswordAttempts` _integer_ | MaxPasswordAttempts is the number of failed password attempts before<br />the account is locked. 0 disables password lockout. |  |  |
| `maxOtpAttempts` _integer_ | MaxOtpAttempts is the number of failed OTP attempts before the account<br />is locked. 0 disables OTP lockout. |  | Optional: \{\} <br /> |


#### DefaultLockoutPolicyStatus



DefaultLockoutPolicyStatus defines the observed state of DefaultLockoutPolicy.



_Appears in:_
- [DefaultLockoutPolicy](#defaultlockoutpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultLockoutPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultLoginPolicy



DefaultLoginPolicy is the Schema for the defaultloginpolicies API.
It manages the instance-level default login policy (IAM_OWNER, Admin API).
Singleton per instance: reconcile reads the current default, diffs all fields, updates on drift.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultLoginPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultLoginPolicySpec](#defaultloginpolicyspec)_ |  |  |  |
| `status` _[DefaultLoginPolicyStatus](#defaultloginpolicystatus)_ |  |  |  |


#### DefaultLoginPolicySpec



DefaultLoginPolicySpec defines the desired state of DefaultLoginPolicy.



_Appears in:_
- [DefaultLoginPolicy](#defaultloginpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userLogin` _boolean_ | UserLogin determines whether login with username/password is allowed. |  | Optional: \{\} <br /> |
| `allowExternalIdp` _boolean_ | AllowExternalIdp allows login with external identity providers. |  | Optional: \{\} <br /> |
| `allowRegister` _boolean_ | AllowRegister allows user self-registration. |  | Optional: \{\} <br /> |
| `forceMfa` _boolean_ | ForceMfa requires multi-factor authentication for all users. |  | Optional: \{\} <br /> |
| `forceMfaLocalOnly` _boolean_ | ForceMfaLocalOnly requires MFA only for local (non-IdP) users. |  | Optional: \{\} <br /> |
| `hidePasswordReset` _boolean_ | HidePasswordReset hides the "forgot password" link on the login page. |  | Optional: \{\} <br /> |
| `passwordlessType` _string_ | PasswordlessType configures passwordless authentication. |  | Enum: [not_allowed allowed] <br />Optional: \{\} <br /> |
| `allowDomainDiscovery` _boolean_ | AllowDomainDiscovery enables domain-based organization discovery. |  | Optional: \{\} <br /> |
| `ignoreUnknownUsernames` _boolean_ | IgnoreUnknownUsernames prevents enumeration attacks by not revealing if a username exists. |  | Optional: \{\} <br /> |
| `defaultRedirectUri` _string_ | DefaultRedirectUri is the default redirect URI after login. |  | Optional: \{\} <br /> |
| `passwordCheckLifetime` _string_ | PasswordCheckLifetime is the duration before password re-verification is required (e.g., "240h"). |  | Optional: \{\} <br /> |
| `externalLoginCheckLifetime` _string_ | ExternalLoginCheckLifetime is the duration before external login re-verification (e.g., "240h"). |  | Optional: \{\} <br /> |
| `mfaInitSkipLifetime` _string_ | MfaInitSkipLifetime is the duration a user can skip MFA setup after login (e.g., "720h"). |  | Optional: \{\} <br /> |
| `multiFactorCheckLifetime` _string_ | MultiFactorCheckLifetime is the duration before MFA re-verification (e.g., "12h"). |  | Optional: \{\} <br /> |
| `secondFactorCheckLifetime` _string_ | SecondFactorCheckLifetime is the duration before second factor re-verification (e.g., "12h"). |  | Optional: \{\} <br /> |
| `idps` _[IdpReference](#idpreference) array_ | Idps is the list of identity providers allowed for login at the instance level. |  | Optional: \{\} <br /> |


#### DefaultLoginPolicyStatus



DefaultLoginPolicyStatus defines the observed state of DefaultLoginPolicy.



_Appears in:_
- [DefaultLoginPolicy](#defaultloginpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultLoginPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultMessageText



DefaultMessageText is the Schema for the defaultmessagetexts API.
It manages instance-level default message text (IAM_OWNER, Admin API). One CR per type+language combination.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultMessageText` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultMessageTextSpec](#defaultmessagetextspec)_ |  |  |  |
| `status` _[DefaultMessageTextStatus](#defaultmessagetextstatus)_ |  |  |  |


#### DefaultMessageTextSpec



DefaultMessageTextSpec defines the desired state of DefaultMessageText.



_Appears in:_
- [DefaultMessageText](#defaultmessagetext)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the message type. |  | Enum: [init passwordReset verifyEmail verifyPhone verifySmsOtp verifyEmailOtp domainClaimed passwordlessRegistration passwordChange inviteUser] <br /> |
| `language` _string_ | Language is the BCP 47 language tag (e.g., "en", "de", "fr"). |  |  |
| `title` _string_ | Title is the message title. |  | Optional: \{\} <br /> |
| `preHeader` _string_ | PreHeader is the email pre-header text. |  | Optional: \{\} <br /> |
| `subject` _string_ | Subject is the email subject line. |  | Optional: \{\} <br /> |
| `greeting` _string_ | Greeting is the greeting line (supports \{\{.FirstName\}\} template). |  | Optional: \{\} <br /> |
| `text` _string_ | Text is the main body text (supports templates). |  | Optional: \{\} <br /> |
| `buttonText` _string_ | ButtonText is the CTA button text. |  | Optional: \{\} <br /> |
| `footerText` _string_ | FooterText is the email footer text. |  | Optional: \{\} <br /> |


#### DefaultMessageTextStatus



DefaultMessageTextStatus defines the observed state of DefaultMessageText.



_Appears in:_
- [DefaultMessageText](#defaultmessagetext)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultMessageText is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultNotificationPolicy



DefaultNotificationPolicy is the Schema for the defaultnotificationpolicies API.
It manages the instance-level default notification policy (IAM_OWNER, Admin API). Singleton.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultNotificationPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultNotificationPolicySpec](#defaultnotificationpolicyspec)_ |  |  |  |
| `status` _[DefaultNotificationPolicyStatus](#defaultnotificationpolicystatus)_ |  |  |  |


#### DefaultNotificationPolicySpec



DefaultNotificationPolicySpec defines the desired state of DefaultNotificationPolicy.



_Appears in:_
- [DefaultNotificationPolicy](#defaultnotificationpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `passwordChange` _boolean_ | PasswordChange determines whether a notification is sent on password change. |  | Optional: \{\} <br /> |


#### DefaultNotificationPolicyStatus



DefaultNotificationPolicyStatus defines the observed state of DefaultNotificationPolicy.



_Appears in:_
- [DefaultNotificationPolicy](#defaultnotificationpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultNotificationPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultOIDCSettings



DefaultOIDCSettings is the Schema for the defaultoidcsettings API.
It manages the instance-level OIDC settings (IAM_OWNER, Admin API). Singleton.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultOIDCSettings` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultOIDCSettingsSpec](#defaultoidcsettingsspec)_ |  |  |  |
| `status` _[DefaultOIDCSettingsStatus](#defaultoidcsettingsstatus)_ |  |  |  |


#### DefaultOIDCSettingsSpec



DefaultOIDCSettingsSpec defines the desired state of DefaultOIDCSettings.



_Appears in:_
- [DefaultOIDCSettings](#defaultoidcsettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accessTokenLifetime` _string_ | AccessTokenLifetime is the duration for access token validity (e.g., "12h"). |  | Optional: \{\} <br /> |
| `idTokenLifetime` _string_ | IdTokenLifetime is the duration for ID token validity (e.g., "12h"). |  | Optional: \{\} <br /> |
| `refreshTokenIdleExpiration` _string_ | RefreshTokenIdleExpiration is the idle expiration for refresh tokens (e.g., "720h"). |  | Optional: \{\} <br /> |
| `refreshTokenExpiration` _string_ | RefreshTokenExpiration is the absolute expiration for refresh tokens (e.g., "2160h"). |  | Optional: \{\} <br /> |


#### DefaultOIDCSettingsStatus



DefaultOIDCSettingsStatus defines the observed state of DefaultOIDCSettings.



_Appears in:_
- [DefaultOIDCSettings](#defaultoidcsettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultOIDCSettings is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultPasswordAgePolicy



DefaultPasswordAgePolicy is the Schema for the defaultpasswordagepolicies API.
It manages the instance-level default password age policy (IAM_OWNER, Admin API). Singleton.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultPasswordAgePolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultPasswordAgePolicySpec](#defaultpasswordagepolicyspec)_ |  |  |  |
| `status` _[DefaultPasswordAgePolicyStatus](#defaultpasswordagepolicystatus)_ |  |  |  |


#### DefaultPasswordAgePolicySpec



DefaultPasswordAgePolicySpec defines the desired state of DefaultPasswordAgePolicy.



_Appears in:_
- [DefaultPasswordAgePolicy](#defaultpasswordagepolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxAgeDays` _integer_ | MaxAgeDays is the maximum number of days a password can be used before it must be changed.<br />0 means no expiration. |  |  |
| `expireWarnDays` _integer_ | ExpireWarnDays is the number of days before expiration to warn the user.<br />0 means no warning. |  | Optional: \{\} <br /> |


#### DefaultPasswordAgePolicyStatus



DefaultPasswordAgePolicyStatus defines the observed state of DefaultPasswordAgePolicy.



_Appears in:_
- [DefaultPasswordAgePolicy](#defaultpasswordagepolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultPasswordAgePolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultPasswordComplexityPolicy



DefaultPasswordComplexityPolicy is the Schema for the defaultpasswordcomplexitypolicies API.
It manages the instance-level default password complexity policy (IAM_OWNER, Admin API). Singleton.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultPasswordComplexityPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultPasswordComplexityPolicySpec](#defaultpasswordcomplexitypolicyspec)_ |  |  |  |
| `status` _[DefaultPasswordComplexityPolicyStatus](#defaultpasswordcomplexitypolicystatus)_ |  |  |  |


#### DefaultPasswordComplexityPolicySpec



DefaultPasswordComplexityPolicySpec defines the desired state of DefaultPasswordComplexityPolicy.



_Appears in:_
- [DefaultPasswordComplexityPolicy](#defaultpasswordcomplexitypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minLength` _integer_ | MinLength is the minimum password length in characters. |  | Minimum: 1 <br /> |
| `hasLowercase` _boolean_ | HasLowercase requires at least one lowercase letter. |  | Optional: \{\} <br /> |
| `hasUppercase` _boolean_ | HasUppercase requires at least one uppercase letter. |  | Optional: \{\} <br /> |
| `hasNumber` _boolean_ | HasNumber requires at least one digit. |  | Optional: \{\} <br /> |
| `hasSymbol` _boolean_ | HasSymbol requires at least one symbol character. |  | Optional: \{\} <br /> |


#### DefaultPasswordComplexityPolicyStatus



DefaultPasswordComplexityPolicyStatus defines the observed state of DefaultPasswordComplexityPolicy.



_Appears in:_
- [DefaultPasswordComplexityPolicy](#defaultpasswordcomplexitypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultPasswordComplexityPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### DefaultPrivacyPolicy



DefaultPrivacyPolicy is the Schema for the defaultprivacypolicies API.
It manages the instance-level default privacy policy (IAM_OWNER, Admin API). Singleton.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `DefaultPrivacyPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DefaultPrivacyPolicySpec](#defaultprivacypolicyspec)_ |  |  |  |
| `status` _[DefaultPrivacyPolicyStatus](#defaultprivacypolicystatus)_ |  |  |  |


#### DefaultPrivacyPolicySpec



DefaultPrivacyPolicySpec defines the desired state of DefaultPrivacyPolicy.



_Appears in:_
- [DefaultPrivacyPolicy](#defaultprivacypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `tosLink` _string_ | TosLink is the URL to the Terms of Service. |  | Optional: \{\} <br /> |
| `privacyLink` _string_ | PrivacyLink is the URL to the Privacy Policy. |  | Optional: \{\} <br /> |
| `helpLink` _string_ | HelpLink is the URL to the Help/Support page. |  | Optional: \{\} <br /> |
| `supportEmail` _string_ | SupportEmail is the support email address. |  | Optional: \{\} <br /> |
| `docsLink` _string_ | DocsLink is the URL to the documentation. |  | Optional: \{\} <br /> |
| `customLink` _string_ | CustomLink is a custom link URL. |  | Optional: \{\} <br /> |
| `customLinkText` _string_ | CustomLinkText is the display text for the custom link. |  | Optional: \{\} <br /> |


#### DefaultPrivacyPolicyStatus



DefaultPrivacyPolicyStatus defines the observed state of DefaultPrivacyPolicy.



_Appears in:_
- [DefaultPrivacyPolicy](#defaultprivacypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the DefaultPrivacyPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### Domain



Domain is the Schema for the domains API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `Domain` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DomainSpec](#domainspec)_ |  |  |  |
| `status` _[DomainStatus](#domainstatus)_ |  |  |  |


#### DomainSpec



DomainSpec defines the desired state of Domain.



_Appears in:_
- [Domain](#domain)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `domainName` _string_ | DomainName is the domain to register for the organization (e.g., "example.com"). |  |  |


#### DomainStatus



DomainStatus defines the observed state of Domain.



_Appears in:_
- [Domain](#domain)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the Domain is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### EmailProvider



EmailProvider is the Schema for the emailproviders API.
It manages an instance-level email provider (IAM_OWNER, Admin API).
Discriminated type: smtp or http.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `EmailProvider` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[EmailProviderSpec](#emailproviderspec)_ |  |  |  |
| `status` _[EmailProviderStatus](#emailproviderstatus)_ |  |  |  |


#### EmailProviderSpec



EmailProviderSpec defines the desired state of EmailProvider.
Discriminated type: exactly one of Smtp or Http must be set.



_Appears in:_
- [EmailProvider](#emailprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `smtp` _[SmtpEmailProvider](#smtpemailprovider)_ | Smtp configures an SMTP email provider.<br />Mutually exclusive with Http. |  | Optional: \{\} <br /> |
| `http` _[HttpEmailProvider](#httpemailprovider)_ | Http configures an HTTP webhook email provider.<br />Mutually exclusive with Smtp. |  | Optional: \{\} <br /> |


#### EmailProviderStatus



EmailProviderStatus defines the observed state of EmailProvider.



_Appears in:_
- [EmailProvider](#emailprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `providerId` _string_ | ProviderId is the Zitadel email provider ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the EmailProvider is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### GitHubIdP



GitHubIdP is the Schema for the githubidps API.
It manages an instance-scoped GitHub identity provider (IAM_OWNER, Admin API).
Uses Admin API AddGitHubProvider/UpdateGitHubProvider.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `GitHubIdP` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[GitHubIdPSpec](#githubidpspec)_ |  |  |  |
| `status` _[GitHubIdPStatus](#githubidpstatus)_ |  |  |  |


#### GitHubIdPSpec



GitHubIdPSpec defines the desired state of GitHubIdP.



_Appears in:_
- [GitHubIdP](#githubidp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the display name of the GitHub identity provider in Zitadel. |  |  |
| `clientId` _string_ | ClientId is the GitHub OAuth2 client ID. |  |  |
| `clientSecretRef` _[SecretKeyRef](#secretkeyref)_ | ClientSecretRef references the Secret containing the GitHub OAuth2 client secret. |  |  |
| `scopes` _string array_ | Scopes are the OAuth2 scopes to request from GitHub. |  | Optional: \{\} <br /> |
| `isLinkingAllowed` _boolean_ | IsLinkingAllowed allows linking existing Zitadel accounts with GitHub accounts. |  | Optional: \{\} <br /> |
| `isCreationAllowed` _boolean_ | IsCreationAllowed allows automatic user creation on first GitHub login. |  | Optional: \{\} <br /> |
| `isAutoCreation` _boolean_ | IsAutoCreation enables automatic user creation (same as IsCreationAllowed in Zitadel API). |  | Optional: \{\} <br /> |
| `isAutoUpdate` _boolean_ | IsAutoUpdate enables automatic user profile update on GitHub login. |  | Optional: \{\} <br /> |


#### GitHubIdPStatus



GitHubIdPStatus defines the observed state of GitHubIdP.



_Appears in:_
- [GitHubIdP](#githubidp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `idpID` _string_ | IdpID is the Zitadel identity provider ID for this GitHub IdP. |  |  |
| `ready` _boolean_ | Ready indicates whether the GitHubIdP is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### GoogleIdP



GoogleIdP is the Schema for the googleidps API.
It manages an instance-scoped Google identity provider (IAM_OWNER, Admin API).
Uses Admin API AddGoogleProvider/UpdateGoogleProvider.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `GoogleIdP` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[GoogleIdPSpec](#googleidpspec)_ |  |  |  |
| `status` _[GoogleIdPStatus](#googleidpstatus)_ |  |  |  |


#### GoogleIdPSpec



GoogleIdPSpec defines the desired state of GoogleIdP.



_Appears in:_
- [GoogleIdP](#googleidp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the display name of the Google identity provider in Zitadel. |  |  |
| `clientId` _string_ | ClientId is the Google OAuth2 client ID. |  |  |
| `clientSecretRef` _[SecretKeyRef](#secretkeyref)_ | ClientSecretRef references the Secret containing the Google OAuth2 client secret. |  |  |
| `scopes` _string array_ | Scopes are the OAuth2 scopes to request from Google. |  | Optional: \{\} <br /> |
| `isLinkingAllowed` _boolean_ | IsLinkingAllowed allows linking existing Zitadel accounts with Google accounts. |  | Optional: \{\} <br /> |
| `isCreationAllowed` _boolean_ | IsCreationAllowed allows automatic user creation on first Google login. |  | Optional: \{\} <br /> |
| `isAutoCreation` _boolean_ | IsAutoCreation enables automatic user creation (same as IsCreationAllowed in Zitadel API). |  | Optional: \{\} <br /> |
| `isAutoUpdate` _boolean_ | IsAutoUpdate enables automatic user profile update on Google login. |  | Optional: \{\} <br /> |


#### GoogleIdPStatus



GoogleIdPStatus defines the observed state of GoogleIdP.



_Appears in:_
- [GoogleIdP](#googleidp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `idpID` _string_ | IdpID is the Zitadel identity provider ID for this Google IdP.<br />Exposed so that DefaultLoginPolicy.idps[].idpRef can resolve it. |  |  |
| `ready` _boolean_ | Ready indicates whether the GoogleIdP is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### HttpEmailProvider



HttpEmailProvider defines HTTP webhook configuration for email delivery.



_Appears in:_
- [EmailProviderSpec](#emailproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `endpoint` _string_ | Endpoint is the HTTP(S) URL that receives email notifications. |  |  |


#### HttpSmsProvider



HttpSmsProvider defines HTTP webhook SMS configuration.



_Appears in:_
- [SmsProviderSpec](#smsproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `endpoint` _string_ | Endpoint is the HTTP(S) URL that receives SMS notifications. |  |  |
| `description` _string_ | Description is a human-readable description. |  | Optional: \{\} <br /> |


#### HumanUser



HumanUser is the Schema for the humanusers API.
It manages an org-scoped human user (Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `HumanUser` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HumanUserSpec](#humanuserspec)_ |  |  |  |
| `status` _[HumanUserStatus](#humanuserstatus)_ |  |  |  |


#### HumanUserSpec



HumanUserSpec defines the desired state of HumanUser.



_Appears in:_
- [HumanUser](#humanuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `userName` _string_ | UserName is the login name of the user. |  |  |
| `firstName` _string_ | FirstName is the user's first name. |  |  |
| `lastName` _string_ | LastName is the user's last name. |  |  |
| `email` _string_ | Email is the user's email address. |  |  |
| `isEmailVerified` _boolean_ | IsEmailVerified marks the email as pre-verified. |  | Optional: \{\} <br /> |
| `displayName` _string_ | DisplayName is the user's display name. |  | Optional: \{\} <br /> |
| `nickName` _string_ | NickName is the user's nickname. |  | Optional: \{\} <br /> |
| `preferredLanguage` _string_ | PreferredLanguage is the user's preferred language (BCP 47). |  | Optional: \{\} <br /> |
| `phone` _string_ | Phone is the user's phone number. |  | Optional: \{\} <br /> |
| `isPhoneVerified` _boolean_ | IsPhoneVerified marks the phone as pre-verified. |  | Optional: \{\} <br /> |
| `initialPasswordSecretRef` _[SecretKeyRef](#secretkeyref)_ | InitialPasswordSecretRef references a Secret containing the initial password. |  | Optional: \{\} <br /> |


#### HumanUserStatus



HumanUserStatus defines the observed state of HumanUser.



_Appears in:_
- [HumanUser](#humanuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userId` _string_ | UserId is the Zitadel user ID. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the HumanUser is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### IdentityProvider



IdentityProvider is the Schema for the identityproviders API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `IdentityProvider` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[IdentityProviderSpec](#identityproviderspec)_ |  |  |  |
| `status` _[IdentityProviderStatus](#identityproviderstatus)_ |  |  |  |


#### IdentityProviderSpec



IdentityProviderSpec defines the desired state of IdentityProvider.



_Appears in:_
- [IdentityProvider](#identityprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `name` _string_ | Name is the display name for the identity provider. |  |  |
| `issuer` _string_ | Issuer is the OIDC issuer URL. |  |  |
| `clientId` _string_ | ClientId is the OIDC client ID. |  |  |
| `clientSecret` _string_ | ClientSecret is the OIDC client secret. |  |  |
| `scopes` _string array_ | Scopes are the OIDC scopes to request. |  | Optional: \{\} <br /> |
| `isAutoCreation` _boolean_ | IsAutoCreation enables automatic user creation on first login. |  | Optional: \{\} <br /> |
| `isAutoUpdate` _boolean_ | IsAutoUpdate enables automatic user profile update on login. |  | Optional: \{\} <br /> |
| `isLinkingAllowed` _boolean_ | IsLinkingAllowed enables account linking. |  | Optional: \{\} <br /> |


#### IdentityProviderStatus



IdentityProviderStatus defines the observed state of IdentityProvider.



_Appears in:_
- [IdentityProvider](#identityprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `idpId` _string_ | IdpId is the Zitadel identity provider ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the IdentityProvider is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### IdpReference



IdpReference references an identity provider either by CR reference or raw ID.



_Appears in:_
- [DefaultLoginPolicySpec](#defaultloginpolicyspec)
- [LoginPolicySpec](#loginpolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `idpRef` _[ResourceRef](#resourceref)_ | IdpRef references a GoogleIdP CR managed by this operator.<br />Mutually exclusive with IdpId. |  | Optional: \{\} <br /> |
| `idpId` _string_ | IdpId references a pre-existing Zitadel identity provider by raw ID.<br />Mutually exclusive with IdpRef. |  | Optional: \{\} <br /> |


#### InstanceMember



InstanceMember is the Schema for the instancemembers API.
It manages an instance-level membership (Admin API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `InstanceMember` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[InstanceMemberSpec](#instancememberspec)_ |  |  |  |
| `status` _[InstanceMemberStatus](#instancememberstatus)_ |  |  |  |


#### InstanceMemberSpec



InstanceMemberSpec defines the desired state of InstanceMember.



_Appears in:_
- [InstanceMember](#instancemember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userRef` _[ResourceRef](#resourceref)_ | UserRef references a MachineUser or HumanUser CR. |  | Optional: \{\} <br /> |
| `userId` _string_ | UserId is a raw Zitadel user ID. |  | Optional: \{\} <br /> |
| `roles` _string array_ | Roles is the list of instance-level roles (e.g., IAM_OWNER, IAM_ORG_MANAGER). |  |  |


#### InstanceMemberStatus



InstanceMemberStatus defines the observed state of InstanceMember.



_Appears in:_
- [InstanceMember](#instancemember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the InstanceMember is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### LabelPolicy



LabelPolicy is the Schema for the labelpolicies API.
It manages an org-scoped label/branding policy (Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `LabelPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[LabelPolicySpec](#labelpolicyspec)_ |  |  |  |
| `status` _[LabelPolicyStatus](#labelpolicystatus)_ |  |  |  |


#### LabelPolicyFields



LabelPolicyFields contains fields shared between LabelPolicy (org) and DefaultLabelPolicy (instance).



_Appears in:_
- [DefaultLabelPolicySpec](#defaultlabelpolicyspec)
- [LabelPolicySpec](#labelpolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `primaryColor` _string_ | PrimaryColor is the primary brand color (hex, e.g. "#5469d4"). |  | Optional: \{\} <br /> |
| `backgroundColor` _string_ | BackgroundColor is the background color (hex). |  | Optional: \{\} <br /> |
| `warnColor` _string_ | WarnColor is the warning color (hex). |  | Optional: \{\} <br /> |
| `fontColor` _string_ | FontColor is the font color (hex). |  | Optional: \{\} <br /> |
| `primaryColorDark` _string_ | PrimaryColorDark is the primary color for dark mode (hex). |  | Optional: \{\} <br /> |
| `backgroundColorDark` _string_ | BackgroundColorDark is the background color for dark mode (hex). |  | Optional: \{\} <br /> |
| `warnColorDark` _string_ | WarnColorDark is the warning color for dark mode (hex). |  | Optional: \{\} <br /> |
| `fontColorDark` _string_ | FontColorDark is the font color for dark mode (hex). |  | Optional: \{\} <br /> |
| `hideLoginNameSuffix` _boolean_ | HideLoginNameSuffix hides the @domain suffix on the login screen. |  | Optional: \{\} <br /> |
| `disableWatermark` _boolean_ | DisableWatermark disables the Zitadel watermark on login pages. |  | Optional: \{\} <br /> |


#### LabelPolicySpec



LabelPolicySpec defines the desired state of LabelPolicy.



_Appears in:_
- [LabelPolicy](#labelpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `primaryColor` _string_ | PrimaryColor is the primary brand color (hex, e.g. "#5469d4"). |  | Optional: \{\} <br /> |
| `backgroundColor` _string_ | BackgroundColor is the background color (hex). |  | Optional: \{\} <br /> |
| `warnColor` _string_ | WarnColor is the warning color (hex). |  | Optional: \{\} <br /> |
| `fontColor` _string_ | FontColor is the font color (hex). |  | Optional: \{\} <br /> |
| `primaryColorDark` _string_ | PrimaryColorDark is the primary color for dark mode (hex). |  | Optional: \{\} <br /> |
| `backgroundColorDark` _string_ | BackgroundColorDark is the background color for dark mode (hex). |  | Optional: \{\} <br /> |
| `warnColorDark` _string_ | WarnColorDark is the warning color for dark mode (hex). |  | Optional: \{\} <br /> |
| `fontColorDark` _string_ | FontColorDark is the font color for dark mode (hex). |  | Optional: \{\} <br /> |
| `hideLoginNameSuffix` _boolean_ | HideLoginNameSuffix hides the @domain suffix on the login screen. |  | Optional: \{\} <br /> |
| `disableWatermark` _boolean_ | DisableWatermark disables the Zitadel watermark on login pages. |  | Optional: \{\} <br /> |


#### LabelPolicyStatus



LabelPolicyStatus defines the observed state of LabelPolicy.



_Appears in:_
- [LabelPolicy](#labelpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the LabelPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### LockoutPolicy



LockoutPolicy is the Schema for the lockoutpolicies API.
It manages an org-scoped lockout policy (ORG_OWNER, Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `LockoutPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[LockoutPolicySpec](#lockoutpolicyspec)_ |  |  |  |
| `status` _[LockoutPolicyStatus](#lockoutpolicystatus)_ |  |  |  |


#### LockoutPolicyFields



LockoutPolicyFields contains fields shared between LockoutPolicy (org) and DefaultLockoutPolicy (instance).



_Appears in:_
- [DefaultLockoutPolicySpec](#defaultlockoutpolicyspec)
- [LockoutPolicySpec](#lockoutpolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxPasswordAttempts` _integer_ | MaxPasswordAttempts is the number of failed password attempts before<br />the account is locked. 0 disables password lockout. |  |  |
| `maxOtpAttempts` _integer_ | MaxOtpAttempts is the number of failed OTP attempts before the account<br />is locked. 0 disables OTP lockout. |  | Optional: \{\} <br /> |


#### LockoutPolicySpec



LockoutPolicySpec defines the desired state of LockoutPolicy (org-scoped).



_Appears in:_
- [LockoutPolicy](#lockoutpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `maxPasswordAttempts` _integer_ | MaxPasswordAttempts is the number of failed password attempts before<br />the account is locked. 0 disables password lockout. |  |  |
| `maxOtpAttempts` _integer_ | MaxOtpAttempts is the number of failed OTP attempts before the account<br />is locked. 0 disables OTP lockout. |  | Optional: \{\} <br /> |


#### LockoutPolicyStatus



LockoutPolicyStatus defines the observed state of LockoutPolicy.



_Appears in:_
- [LockoutPolicy](#lockoutpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the LockoutPolicy is successfully synced. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### LoginPolicy



LoginPolicy is the Schema for the loginpolicies API.
It manages an org-scoped login policy (ORG_OWNER, Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `LoginPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[LoginPolicySpec](#loginpolicyspec)_ |  |  |  |
| `status` _[LoginPolicyStatus](#loginpolicystatus)_ |  |  |  |


#### LoginPolicyFields



LoginPolicyFields contains fields shared between LoginPolicy (org) and DefaultLoginPolicy (instance).
Unset (nil) booleans leave the corresponding Zitadel setting unchanged.



_Appears in:_
- [LoginPolicySpec](#loginpolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userLogin` _boolean_ | UserLogin allows username/password login. |  |  |
| `allowExternalIdp` _boolean_ | AllowExternalIdp allows login through configured external identity providers. |  |  |
| `allowRegister` _boolean_ | AllowRegister allows user self-registration. |  |  |
| `forceMfa` _boolean_ | ForceMfa requires multi-factor authentication for all users. |  |  |
| `forceMfaLocalOnly` _boolean_ | ForceMfaLocalOnly requires MFA only for local (non-IdP) authentication. |  |  |
| `hidePasswordReset` _boolean_ | HidePasswordReset hides the password-reset link on the login form. |  |  |
| `passwordlessType` _string_ | PasswordlessType enables passwordless (passkey) login. |  | Enum: [not_allowed allowed] <br />Optional: \{\} <br /> |
| `allowDomainDiscovery` _boolean_ | AllowDomainDiscovery enables org discovery by the domain suffix of the login name. |  |  |
| `ignoreUnknownUsernames` _boolean_ | IgnoreUnknownUsernames shows the password step even for unknown users (user-enumeration hardening). |  |  |
| `defaultRedirectUri` _string_ | DefaultRedirectUri is where users land after login when no app context exists. |  |  |
| `passwordCheckLifetime` _string_ | PasswordCheckLifetime is how long a password check stays valid (Go/Zitadel duration string, e.g. "240h"). |  |  |
| `externalLoginCheckLifetime` _string_ | ExternalLoginCheckLifetime is how long an external IdP login stays valid (duration string). |  |  |
| `mfaInitSkipLifetime` _string_ | MfaInitSkipLifetime is how long users may postpone MFA setup (duration string; "0s" = no skip). |  |  |
| `multiFactorCheckLifetime` _string_ | MultiFactorCheckLifetime is how long a multi-factor check stays valid (duration string). |  |  |
| `secondFactorCheckLifetime` _string_ | SecondFactorCheckLifetime is how long a second-factor check stays valid (duration string). |  |  |


#### LoginPolicySpec



LoginPolicySpec defines the desired state of LoginPolicy (org-scoped).



_Appears in:_
- [LoginPolicy](#loginpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `userLogin` _boolean_ | UserLogin allows username/password login. |  |  |
| `allowExternalIdp` _boolean_ | AllowExternalIdp allows login through configured external identity providers. |  |  |
| `allowRegister` _boolean_ | AllowRegister allows user self-registration. |  |  |
| `forceMfa` _boolean_ | ForceMfa requires multi-factor authentication for all users. |  |  |
| `forceMfaLocalOnly` _boolean_ | ForceMfaLocalOnly requires MFA only for local (non-IdP) authentication. |  |  |
| `hidePasswordReset` _boolean_ | HidePasswordReset hides the password-reset link on the login form. |  |  |
| `passwordlessType` _string_ | PasswordlessType enables passwordless (passkey) login. |  | Enum: [not_allowed allowed] <br />Optional: \{\} <br /> |
| `allowDomainDiscovery` _boolean_ | AllowDomainDiscovery enables org discovery by the domain suffix of the login name. |  |  |
| `ignoreUnknownUsernames` _boolean_ | IgnoreUnknownUsernames shows the password step even for unknown users (user-enumeration hardening). |  |  |
| `defaultRedirectUri` _string_ | DefaultRedirectUri is where users land after login when no app context exists. |  |  |
| `passwordCheckLifetime` _string_ | PasswordCheckLifetime is how long a password check stays valid (Go/Zitadel duration string, e.g. "240h"). |  |  |
| `externalLoginCheckLifetime` _string_ | ExternalLoginCheckLifetime is how long an external IdP login stays valid (duration string). |  |  |
| `mfaInitSkipLifetime` _string_ | MfaInitSkipLifetime is how long users may postpone MFA setup (duration string; "0s" = no skip). |  |  |
| `multiFactorCheckLifetime` _string_ | MultiFactorCheckLifetime is how long a multi-factor check stays valid (duration string). |  |  |
| `secondFactorCheckLifetime` _string_ | SecondFactorCheckLifetime is how long a second-factor check stays valid (duration string). |  |  |
| `disableLoginWithEmail` _boolean_ | DisableLoginWithEmail disables login using email addresses. |  | Optional: \{\} <br /> |
| `disableLoginWithPhone` _boolean_ | DisableLoginWithPhone disables login using phone numbers. |  | Optional: \{\} <br /> |
| `secondFactors` _string array_ | SecondFactors is the list of allowed second factor types. |  | Optional: \{\} <br /> |
| `multiFactors` _string array_ | MultiFactors is the list of allowed multi-factor types. |  | Optional: \{\} <br /> |
| `idps` _[IdpReference](#idpreference) array_ | Idps is the list of identity providers allowed for login at the org level. |  | Optional: \{\} <br /> |


#### LoginPolicyStatus



LoginPolicyStatus defines the observed state of LoginPolicy.



_Appears in:_
- [LoginPolicy](#loginpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the LoginPolicy is successfully synced. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### MachineKeySecretRef



MachineKeySecretRef references a Secret where the machine user key JSON will be stored.



_Appears in:_
- [ApplicationKeySpec](#applicationkeyspec)
- [MachineUserSpec](#machineuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Secret. |  |  |
| `key` _string_ | Key is the data key within the Secret. Default: "key.json" |  | Optional: \{\} <br /> |


#### MachineKeySpec



MachineKeySpec configures machine key rotation.



_Appears in:_
- [MachineUserSpec](#machineuserspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `rotateAfter` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | RotateAfter enables dual-key rotation: once the current key is older<br />than this duration, a new key is minted and swapped into the Secret;<br />the old key is revoked after RotationGrace (two keys coexist during<br />the overlap). Example: "2160h" (90 days). Empty = never rotate<br />(pre-v0.18 behavior). |  | Optional: \{\} <br /> |
| `rotationGrace` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta)_ | RotationGrace is how long the old key stays valid after rotation.<br />Defaults to 5m. |  | Optional: \{\} <br /> |


#### MachineUser



MachineUser is the Schema for the machineusers API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `MachineUser` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[MachineUserSpec](#machineuserspec)_ |  |  |  |
| `status` _[MachineUserStatus](#machineuserstatus)_ |  |  |  |


#### MachineUserSpec



MachineUserSpec defines the desired state of MachineUser.



_Appears in:_
- [MachineUser](#machineuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `userName` _string_ | UserName is the login name for the machine user. |  |  |
| `name` _string_ | Name is the display name of the machine user.<br />If empty, the Kubernetes resource name is used. |  | Optional: \{\} <br /> |
| `description` _string_ | Description is an optional description of the machine user. |  | Optional: \{\} <br /> |
| `accessTokenType` _string_ | AccessTokenType specifies the access token format for this machine user. |  | Enum: [bearer jwt] <br />Optional: \{\} <br /> |
| `roles` _string array_ | Roles are project role grants for this machine user (v0.18, INF-426).<br />The grant target is, in order of precedence: the project named by<br />ProjectRef/ProjectId (v0.19), the namespace's resolved scope project<br />(scope maps), or a previously recorded status.projectId. Grants never<br />widen beyond the resolved scope when scope maps are active. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR whose project the role grant is<br />created on (v0.19 fleet shape: declare the project in-namespace and<br />grant roles on it without scope maps). Only used with Roles.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID as the<br />role grant target. Only used with Roles.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `key` _[MachineKeySpec](#machinekeyspec)_ | Key configures machine key lifecycle (v0.18, INF-426). |  | Optional: \{\} <br /> |
| `keySecretRef` _[MachineKeySecretRef](#machinekeysecretref)_ | KeySecretRef references the Secret where the generated key JSON will be stored. |  |  |


#### MachineUserStatus



MachineUserStatus defines the observed state of MachineUser.



_Appears in:_
- [MachineUser](#machineuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userId` _string_ | UserId is the Zitadel user ID. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `projectId` _string_ | ProjectId is the scope project the roles are granted on (v0.18). |  |  |
| `grantId` _string_ | GrantId is the Zitadel user grant carrying spec.roles (v0.18). |  |  |
| `keyId` _string_ | KeyId is the current machine key (v0.18 rotation bookkeeping). |  |  |
| `keyCreatedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | KeyCreatedAt is when the current key was minted. |  |  |
| `previousKeyId` _string_ | PreviousKeyId is the rotated-out key awaiting revocation. |  |  |
| `previousKeyRevokeAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | PreviousKeyRevokeAt is when the rotated-out key gets revoked. |  |  |
| `ready` _boolean_ | Ready indicates whether the MachineUser is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### MessageText



MessageText is the Schema for the messagetexts API.
It manages org-scoped custom message text (ORG_OWNER, Management API). One CR per type+language+org combination.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `MessageText` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[MessageTextSpec](#messagetextspec)_ |  |  |  |
| `status` _[MessageTextStatus](#messagetextstatus)_ |  |  |  |


#### MessageTextFields



MessageTextFields contains the text fields shared by all message text types.



_Appears in:_
- [DefaultMessageTextSpec](#defaultmessagetextspec)
- [MessageTextSpec](#messagetextspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the message type. |  | Enum: [init passwordReset verifyEmail verifyPhone verifySmsOtp verifyEmailOtp domainClaimed passwordlessRegistration passwordChange inviteUser] <br /> |
| `language` _string_ | Language is the BCP 47 language tag (e.g., "en", "de", "fr"). |  |  |
| `title` _string_ | Title is the message title. |  | Optional: \{\} <br /> |
| `preHeader` _string_ | PreHeader is the email pre-header text. |  | Optional: \{\} <br /> |
| `subject` _string_ | Subject is the email subject line. |  | Optional: \{\} <br /> |
| `greeting` _string_ | Greeting is the greeting line (supports \{\{.FirstName\}\} template). |  | Optional: \{\} <br /> |
| `text` _string_ | Text is the main body text (supports templates). |  | Optional: \{\} <br /> |
| `buttonText` _string_ | ButtonText is the CTA button text. |  | Optional: \{\} <br /> |
| `footerText` _string_ | FooterText is the email footer text. |  | Optional: \{\} <br /> |


#### MessageTextSpec



MessageTextSpec defines the desired state of MessageText.



_Appears in:_
- [MessageText](#messagetext)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `type` _string_ | Type is the message type. |  | Enum: [init passwordReset verifyEmail verifyPhone verifySmsOtp verifyEmailOtp domainClaimed passwordlessRegistration passwordChange inviteUser] <br /> |
| `language` _string_ | Language is the BCP 47 language tag (e.g., "en", "de", "fr"). |  |  |
| `title` _string_ | Title is the message title. |  | Optional: \{\} <br /> |
| `preHeader` _string_ | PreHeader is the email pre-header text. |  | Optional: \{\} <br /> |
| `subject` _string_ | Subject is the email subject line. |  | Optional: \{\} <br /> |
| `greeting` _string_ | Greeting is the greeting line (supports \{\{.FirstName\}\} template). |  | Optional: \{\} <br /> |
| `text` _string_ | Text is the main body text (supports templates). |  | Optional: \{\} <br /> |
| `buttonText` _string_ | ButtonText is the CTA button text. |  | Optional: \{\} <br /> |
| `footerText` _string_ | FooterText is the email footer text. |  | Optional: \{\} <br /> |


#### MessageTextStatus



MessageTextStatus defines the observed state of MessageText.



_Appears in:_
- [MessageText](#messagetext)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the MessageText is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### NotificationPolicy



NotificationPolicy is the Schema for the notificationpolicies API.
It manages an org-scoped notification policy (Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `NotificationPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[NotificationPolicySpec](#notificationpolicyspec)_ |  |  |  |
| `status` _[NotificationPolicyStatus](#notificationpolicystatus)_ |  |  |  |


#### NotificationPolicyFields



NotificationPolicyFields contains fields shared between NotificationPolicy (org) and DefaultNotificationPolicy (instance).



_Appears in:_
- [DefaultNotificationPolicySpec](#defaultnotificationpolicyspec)
- [NotificationPolicySpec](#notificationpolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `passwordChange` _boolean_ | PasswordChange determines whether a notification is sent on password change. |  | Optional: \{\} <br /> |


#### NotificationPolicySpec



NotificationPolicySpec defines the desired state of NotificationPolicy.



_Appears in:_
- [NotificationPolicy](#notificationpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `passwordChange` _boolean_ | PasswordChange determines whether a notification is sent on password change. |  | Optional: \{\} <br /> |


#### NotificationPolicyStatus



NotificationPolicyStatus defines the observed state of NotificationPolicy.



_Appears in:_
- [NotificationPolicy](#notificationpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the NotificationPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### OIDCApp



OIDCApp is the Schema for the oidcapps API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `OIDCApp` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OIDCAppSpec](#oidcappspec)_ |  |  |  |
| `status` _[OIDCAppStatus](#oidcappstatus)_ |  |  |  |


#### OIDCAppSpec



OIDCAppSpec defines the desired state of OIDCApp.



_Appears in:_
- [OIDCApp](#oidcapp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `name` _string_ | Name is the display name of the application in Zitadel.<br />If empty, the Kubernetes resource name is used. |  | Optional: \{\} <br /> |
| `type` _string_ | Type is the OIDC application type. |  | Enum: [confidential public] <br /> |
| `authMethod` _string_ | AuthMethod is the authentication method. |  | Enum: [basic none] <br /> |
| `redirectUris` _string array_ | RedirectUris is the exact, ordered list of allowed OAuth2/OIDC redirect<br />URIs. The operator is the single writer: server-side drift (added or<br />removed entries) is reverted to this list on every reconcile. Zitadel<br />requires https:// URIs unless the app has dev mode enabled. |  |  |
| `postLogoutRedirectUris` _string array_ | PostLogoutRedirectUris is the list of allowed URIs to return users to<br />after logout (RP-initiated logout). Drift-corrected like RedirectUris. |  | Optional: \{\} <br /> |
| `accessTokenType` _string_ | AccessTokenType selects the access token format: "bearer" (opaque,<br />default) or "jwt" (self-contained, locally verifiable). |  | Enum: [bearer jwt] <br />Optional: \{\} <br /> |
| `accessTokenRoleAssertion` _boolean_ | AccessTokenRoleAssertion includes the user's project role claims in<br />the access token. |  | Optional: \{\} <br /> |
| `idTokenRoleAssertion` _boolean_ | IdTokenRoleAssertion includes the user's project role claims in the ID token. |  | Optional: \{\} <br /> |
| `secretRef` _[SecretRefSpec](#secretrefspec)_ | SecretRef references the Secret where the client credentials will be stored. |  |  |


#### OIDCAppStatus



OIDCAppStatus defines the observed state of OIDCApp.



_Appears in:_
- [OIDCApp](#oidcapp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applicationId` _string_ | ApplicationId is the Zitadel application ID. |  |  |
| `clientId` _string_ | ClientId is the OIDC client ID assigned by Zitadel. |  |  |
| `projectId` _string_ | ProjectId is the resolved project ID this app belongs to. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID (inherited from project). |  |  |
| `ready` _boolean_ | Ready indicates whether the OIDCApp is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### OIDCSettingsFields



OIDCSettingsFields contains fields for DefaultOIDCSettings.



_Appears in:_
- [DefaultOIDCSettingsSpec](#defaultoidcsettingsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `accessTokenLifetime` _string_ | AccessTokenLifetime is the duration for access token validity (e.g., "12h"). |  | Optional: \{\} <br /> |
| `idTokenLifetime` _string_ | IdTokenLifetime is the duration for ID token validity (e.g., "12h"). |  | Optional: \{\} <br /> |
| `refreshTokenIdleExpiration` _string_ | RefreshTokenIdleExpiration is the idle expiration for refresh tokens (e.g., "720h"). |  | Optional: \{\} <br /> |
| `refreshTokenExpiration` _string_ | RefreshTokenExpiration is the absolute expiration for refresh tokens (e.g., "2160h"). |  | Optional: \{\} <br /> |


#### OrgMember



OrgMember is the Schema for the orgmembers API.
It manages an org-scoped membership (Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `OrgMember` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OrgMemberSpec](#orgmemberspec)_ |  |  |  |
| `status` _[OrgMemberStatus](#orgmemberstatus)_ |  |  |  |


#### OrgMemberSpec



OrgMemberSpec defines the desired state of OrgMember.



_Appears in:_
- [OrgMember](#orgmember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `userRef` _[ResourceRef](#resourceref)_ | UserRef references a MachineUser or HumanUser CR. |  | Optional: \{\} <br /> |
| `userId` _string_ | UserId is a raw Zitadel user ID. |  | Optional: \{\} <br /> |
| `roles` _string array_ | Roles is the list of roles to assign (e.g., ORG_OWNER, ORG_ADMIN). |  |  |


#### OrgMemberStatus



OrgMemberStatus defines the observed state of OrgMember.



_Appears in:_
- [OrgMember](#orgmember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the OrgMember is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### OrgMetadata



OrgMetadata is the Schema for the orgmetadata API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `OrgMetadata` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OrgMetadataSpec](#orgmetadataspec)_ |  |  |  |
| `status` _[OrgMetadataStatus](#orgmetadatastatus)_ |  |  |  |


#### OrgMetadataSpec



OrgMetadataSpec defines the desired state of OrgMetadata.



_Appears in:_
- [OrgMetadata](#orgmetadata)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `key` _string_ | Key is the metadata key. |  |  |
| `value` _string_ | Value is the metadata value (will be stored as bytes in Zitadel). |  |  |


#### OrgMetadataStatus



OrgMetadataStatus defines the observed state of OrgMetadata.



_Appears in:_
- [OrgMetadata](#orgmetadata)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the OrgMetadata is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### Organization



Organization is the Schema for the organizations API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `Organization` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OrganizationSpec](#organizationspec)_ |  |  |  |
| `status` _[OrganizationStatus](#organizationstatus)_ |  |  |  |


#### OrganizationSpec



OrganizationSpec defines the desired state of Organization.



_Appears in:_
- [Organization](#organization)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the display name of the organization in Zitadel.<br />If empty, the Kubernetes resource name is used. |  | Optional: \{\} <br /> |


#### OrganizationStatus



OrganizationStatus defines the observed state of Organization.



_Appears in:_
- [Organization](#organization)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organizationId` _string_ | OrganizationId is the Zitadel organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the Organization is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### PasswordAgePolicy



PasswordAgePolicy is the Schema for the passwordagepolicies API.
It manages an org-scoped password age policy (Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `PasswordAgePolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PasswordAgePolicySpec](#passwordagepolicyspec)_ |  |  |  |
| `status` _[PasswordAgePolicyStatus](#passwordagepolicystatus)_ |  |  |  |


#### PasswordAgePolicyFields



PasswordAgePolicyFields contains fields shared between PasswordAgePolicy (org) and DefaultPasswordAgePolicy (instance).



_Appears in:_
- [DefaultPasswordAgePolicySpec](#defaultpasswordagepolicyspec)
- [PasswordAgePolicySpec](#passwordagepolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxAgeDays` _integer_ | MaxAgeDays is the maximum number of days a password can be used before it must be changed.<br />0 means no expiration. |  |  |
| `expireWarnDays` _integer_ | ExpireWarnDays is the number of days before expiration to warn the user.<br />0 means no warning. |  | Optional: \{\} <br /> |


#### PasswordAgePolicySpec



PasswordAgePolicySpec defines the desired state of PasswordAgePolicy.



_Appears in:_
- [PasswordAgePolicy](#passwordagepolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `maxAgeDays` _integer_ | MaxAgeDays is the maximum number of days a password can be used before it must be changed.<br />0 means no expiration. |  |  |
| `expireWarnDays` _integer_ | ExpireWarnDays is the number of days before expiration to warn the user.<br />0 means no warning. |  | Optional: \{\} <br /> |


#### PasswordAgePolicyStatus



PasswordAgePolicyStatus defines the observed state of PasswordAgePolicy.



_Appears in:_
- [PasswordAgePolicy](#passwordagepolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the PasswordAgePolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### PasswordComplexityPolicy



PasswordComplexityPolicy is the Schema for the passwordcomplexitypolicies API.
It manages an org-scoped password complexity policy (ORG_OWNER, Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `PasswordComplexityPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PasswordComplexityPolicySpec](#passwordcomplexitypolicyspec)_ |  |  |  |
| `status` _[PasswordComplexityPolicyStatus](#passwordcomplexitypolicystatus)_ |  |  |  |


#### PasswordComplexityPolicyFields



PasswordComplexityPolicyFields contains fields shared between PasswordComplexityPolicy (org) and DefaultPasswordComplexityPolicy (instance).



_Appears in:_
- [DefaultPasswordComplexityPolicySpec](#defaultpasswordcomplexitypolicyspec)
- [PasswordComplexityPolicySpec](#passwordcomplexitypolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minLength` _integer_ | MinLength is the minimum password length in characters. |  | Minimum: 1 <br /> |
| `hasLowercase` _boolean_ | HasLowercase requires at least one lowercase letter. |  | Optional: \{\} <br /> |
| `hasUppercase` _boolean_ | HasUppercase requires at least one uppercase letter. |  | Optional: \{\} <br /> |
| `hasNumber` _boolean_ | HasNumber requires at least one digit. |  | Optional: \{\} <br /> |
| `hasSymbol` _boolean_ | HasSymbol requires at least one symbol character. |  | Optional: \{\} <br /> |


#### PasswordComplexityPolicySpec



PasswordComplexityPolicySpec defines the desired state of PasswordComplexityPolicy (org-scoped).



_Appears in:_
- [PasswordComplexityPolicy](#passwordcomplexitypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `minLength` _integer_ | MinLength is the minimum password length in characters. |  | Minimum: 1 <br /> |
| `hasLowercase` _boolean_ | HasLowercase requires at least one lowercase letter. |  | Optional: \{\} <br /> |
| `hasUppercase` _boolean_ | HasUppercase requires at least one uppercase letter. |  | Optional: \{\} <br /> |
| `hasNumber` _boolean_ | HasNumber requires at least one digit. |  | Optional: \{\} <br /> |
| `hasSymbol` _boolean_ | HasSymbol requires at least one symbol character. |  | Optional: \{\} <br /> |


#### PasswordComplexityPolicyStatus



PasswordComplexityPolicyStatus defines the observed state of PasswordComplexityPolicy.



_Appears in:_
- [PasswordComplexityPolicy](#passwordcomplexitypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the PasswordComplexityPolicy is successfully synced. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### PersonalAccessToken



PersonalAccessToken is the Schema for the personalaccesstokens API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `PersonalAccessToken` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PersonalAccessTokenSpec](#personalaccesstokenspec)_ |  |  |  |
| `status` _[PersonalAccessTokenStatus](#personalaccesstokenstatus)_ |  |  |  |


#### PersonalAccessTokenSpec



PersonalAccessTokenSpec defines the desired state of PersonalAccessToken.



_Appears in:_
- [PersonalAccessToken](#personalaccesstoken)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `userRef` _[ResourceRef](#resourceref)_ | UserRef references a MachineUser CR managed by this operator.<br />Mutually exclusive with UserId. |  | Optional: \{\} <br /> |
| `userId` _string_ | UserId references a pre-existing Zitadel user by raw ID.<br />Mutually exclusive with UserRef. |  | Optional: \{\} <br /> |
| `expirationDate` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | ExpirationDate is the optional expiration timestamp for the token.<br />If not set, a 10-year expiration is used. |  | Optional: \{\} <br /> |
| `tokenSecretRef` _[TokenSecretRefSpec](#tokensecretrefspec)_ | TokenSecretRef references the Secret where the PAT will be stored. |  |  |


#### PersonalAccessTokenStatus



PersonalAccessTokenStatus defines the observed state of PersonalAccessToken.



_Appears in:_
- [PersonalAccessToken](#personalaccesstoken)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `tokenId` _string_ | TokenId is the Zitadel personal access token ID. |  |  |
| `userId` _string_ | UserId is the resolved user ID. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the PersonalAccessToken is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### PrivacyPolicy



PrivacyPolicy is the Schema for the privacypolicies API.
It manages an org-scoped privacy policy (Management API).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `PrivacyPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PrivacyPolicySpec](#privacypolicyspec)_ |  |  |  |
| `status` _[PrivacyPolicyStatus](#privacypolicystatus)_ |  |  |  |


#### PrivacyPolicyFields



PrivacyPolicyFields contains fields shared between PrivacyPolicy (org) and DefaultPrivacyPolicy (instance).



_Appears in:_
- [DefaultPrivacyPolicySpec](#defaultprivacypolicyspec)
- [PrivacyPolicySpec](#privacypolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `tosLink` _string_ | TosLink is the URL to the Terms of Service. |  | Optional: \{\} <br /> |
| `privacyLink` _string_ | PrivacyLink is the URL to the Privacy Policy. |  | Optional: \{\} <br /> |
| `helpLink` _string_ | HelpLink is the URL to the Help/Support page. |  | Optional: \{\} <br /> |
| `supportEmail` _string_ | SupportEmail is the support email address. |  | Optional: \{\} <br /> |
| `docsLink` _string_ | DocsLink is the URL to the documentation. |  | Optional: \{\} <br /> |
| `customLink` _string_ | CustomLink is a custom link URL. |  | Optional: \{\} <br /> |
| `customLinkText` _string_ | CustomLinkText is the display text for the custom link. |  | Optional: \{\} <br /> |


#### PrivacyPolicySpec



PrivacyPolicySpec defines the desired state of PrivacyPolicy.



_Appears in:_
- [PrivacyPolicy](#privacypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `tosLink` _string_ | TosLink is the URL to the Terms of Service. |  | Optional: \{\} <br /> |
| `privacyLink` _string_ | PrivacyLink is the URL to the Privacy Policy. |  | Optional: \{\} <br /> |
| `helpLink` _string_ | HelpLink is the URL to the Help/Support page. |  | Optional: \{\} <br /> |
| `supportEmail` _string_ | SupportEmail is the support email address. |  | Optional: \{\} <br /> |
| `docsLink` _string_ | DocsLink is the URL to the documentation. |  | Optional: \{\} <br /> |
| `customLink` _string_ | CustomLink is a custom link URL. |  | Optional: \{\} <br /> |
| `customLinkText` _string_ | CustomLinkText is the display text for the custom link. |  | Optional: \{\} <br /> |


#### PrivacyPolicyStatus



PrivacyPolicyStatus defines the observed state of PrivacyPolicy.



_Appears in:_
- [PrivacyPolicy](#privacypolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organizationId` _string_ | OrganizationId is the resolved organization ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the PrivacyPolicy is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### Project



Project is the Schema for the projects API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `Project` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ProjectSpec](#projectspec)_ |  |  |  |
| `status` _[ProjectStatus](#projectstatus)_ |  |  |  |


#### ProjectGrant



ProjectGrant is the Schema for the projectgrants API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ProjectGrant` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ProjectGrantSpec](#projectgrantspec)_ |  |  |  |
| `status` _[ProjectGrantStatus](#projectgrantstatus)_ |  |  |  |


#### ProjectGrantMember



ProjectGrantMember is the Schema for the projectgrantmembers API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ProjectGrantMember` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ProjectGrantMemberSpec](#projectgrantmemberspec)_ |  |  |  |
| `status` _[ProjectGrantMemberStatus](#projectgrantmemberstatus)_ |  |  |  |


#### ProjectGrantMemberSpec



ProjectGrantMemberSpec defines the desired state of ProjectGrantMember.



_Appears in:_
- [ProjectGrantMember](#projectgrantmember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `grantId` _string_ | GrantId is the Zitadel project grant ID (from ProjectGrant status). |  |  |
| `userRef` _[ResourceRef](#resourceref)_ | UserRef references a MachineUser CR managed by this operator.<br />Mutually exclusive with UserId. |  | Optional: \{\} <br /> |
| `userId` _string_ | UserId references a pre-existing Zitadel user by raw ID.<br />Mutually exclusive with UserRef. |  | Optional: \{\} <br /> |
| `roles` _string array_ | Roles is the list of roles to assign to the grant member. |  |  |


#### ProjectGrantMemberStatus



ProjectGrantMemberStatus defines the observed state of ProjectGrantMember.



_Appears in:_
- [ProjectGrantMember](#projectgrantmember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the ProjectGrantMember is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ProjectGrantSpec



ProjectGrantSpec defines the desired state of ProjectGrant.



_Appears in:_
- [ProjectGrant](#projectgrant)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `grantedOrgRef` _[ResourceRef](#resourceref)_ | GrantedOrgRef references an Organization CR that receives the grant.<br />Mutually exclusive with GrantedOrgId. |  | Optional: \{\} <br /> |
| `grantedOrgId` _string_ | GrantedOrgId is the raw Zitadel org ID that receives the grant.<br />Mutually exclusive with GrantedOrgRef. |  | Optional: \{\} <br /> |
| `roleKeys` _string array_ | RoleKeys is the list of role keys to grant to the target org. |  |  |


#### ProjectGrantStatus



ProjectGrantStatus defines the observed state of ProjectGrant.



_Appears in:_
- [ProjectGrant](#projectgrant)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `grantId` _string_ | GrantId is the Zitadel project grant ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the ProjectGrant is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ProjectMember



ProjectMember is the Schema for the projectmembers API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ProjectMember` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ProjectMemberSpec](#projectmemberspec)_ |  |  |  |
| `status` _[ProjectMemberStatus](#projectmemberstatus)_ |  |  |  |


#### ProjectMemberSpec



ProjectMemberSpec defines the desired state of ProjectMember.



_Appears in:_
- [ProjectMember](#projectmember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `userRef` _[ResourceRef](#resourceref)_ | UserRef references a MachineUser CR managed by this operator.<br />Mutually exclusive with UserId. |  | Optional: \{\} <br /> |
| `userId` _string_ | UserId is the Zitadel user ID to add as a project member.<br />Mutually exclusive with UserRef. |  | Optional: \{\} <br /> |
| `roles` _string array_ | Roles is the list of roles to assign (e.g., PROJECT_OWNER, PROJECT_OWNER_VIEWER). |  |  |


#### ProjectMemberStatus



ProjectMemberStatus defines the observed state of ProjectMember.



_Appears in:_
- [ProjectMember](#projectmember)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates whether the ProjectMember is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ProjectRole



ProjectRole is the Schema for the projectroles API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ProjectRole` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ProjectRoleSpec](#projectrolespec)_ |  |  |  |
| `status` _[ProjectRoleStatus](#projectrolestatus)_ |  |  |  |


#### ProjectRoleSpec



ProjectRoleSpec defines the desired state of ProjectRole.



_Appears in:_
- [ProjectRole](#projectrole)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR in the same namespace (or another<br />namespace via ref.namespace). The role is created in that project once<br />it reports a projectId in status.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `key` _string_ | Key is the role key used in authorization checks and token claims.<br />If empty, the Kubernetes resource name is used. The key cannot be<br />changed in Zitadel; changing it here removes the old role (including<br />its grants) and creates a new one. |  | Optional: \{\} <br /> |
| `displayName` _string_ | DisplayName is the human-readable name of the role.<br />If empty, the role key is used. |  | Optional: \{\} <br /> |
| `group` _string_ | Group optionally groups roles for display purposes (not a collection<br />of users; Zitadel does not evaluate it). |  | Optional: \{\} <br /> |


#### ProjectRoleStatus



ProjectRoleStatus defines the observed state of ProjectRole.



_Appears in:_
- [ProjectRole](#projectrole)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `projectId` _string_ | ProjectId is the resolved Zitadel project ID the role belongs to. |  |  |
| `key` _string_ | Key is the role key currently reconciled into the project. Kept so a<br />key change can remove the previously managed role. |  |  |
| `observedGeneration` _integer_ | ObservedGeneration is the spec generation last reconciled. |  |  |
| `ready` _boolean_ | Ready indicates whether the ProjectRole is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ProjectSpec



ProjectSpec defines the desired state of Project.



_Appears in:_
- [Project](#project)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `name` _string_ | Name is the display name of the project in Zitadel.<br />If empty, the Kubernetes resource name is used. |  | Optional: \{\} <br /> |
| `assertRolesOnAuth` _boolean_ | AssertRolesOnAuth determines whether roles are asserted on authentication. |  | Optional: \{\} <br /> |
| `checkAuthorizationOnAuth` _boolean_ | CheckAuthorizationOnAuth enables authorization check on authentication.<br />When true, only users with explicit role assignments can authenticate,<br />and user grants are loaded into the Action context (ctx.v1.user.grants). |  | Optional: \{\} <br /> |
| `roles` _string array_ | Roles is the authoritative full set of role keys for this project:<br />missing roles are added and extra roles are removed on every sync.<br />Prefer ProjectRole CRs (v0.19) for incremental, per-role management —<br />do not combine spec.roles with ProjectRole CRs targeting the same<br />project, or the full-set sync will remove the roles they manage. |  | Optional: \{\} <br /> |


#### ProjectStatus



ProjectStatus defines the observed state of Project.



_Appears in:_
- [Project](#project)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `projectId` _string_ | ProjectId is the Zitadel project ID. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID this project belongs to. |  |  |
| `observedGeneration` _integer_ | ObservedGeneration is the spec generation last reconciled. |  |  |
| `ready` _boolean_ | Ready indicates whether the Project is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ResourceRef



ResourceRef references a Kubernetes resource by name and optional namespace.
If namespace is omitted, defaults to the same namespace as the referencing resource.



_Appears in:_
- [APIAppSpec](#apiappspec)
- [ActionExecutionTarget](#actionexecutiontarget)
- [ApplicationKeySpec](#applicationkeyspec)
- [DomainSpec](#domainspec)
- [HumanUserSpec](#humanuserspec)
- [IdentityProviderSpec](#identityproviderspec)
- [IdpReference](#idpreference)
- [InstanceMemberSpec](#instancememberspec)
- [LabelPolicySpec](#labelpolicyspec)
- [LockoutPolicySpec](#lockoutpolicyspec)
- [LoginPolicySpec](#loginpolicyspec)
- [MachineUserSpec](#machineuserspec)
- [MessageTextSpec](#messagetextspec)
- [NotificationPolicySpec](#notificationpolicyspec)
- [OIDCAppSpec](#oidcappspec)
- [OrgMemberSpec](#orgmemberspec)
- [OrgMetadataSpec](#orgmetadataspec)
- [PasswordAgePolicySpec](#passwordagepolicyspec)
- [PasswordComplexityPolicySpec](#passwordcomplexitypolicyspec)
- [PersonalAccessTokenSpec](#personalaccesstokenspec)
- [PrivacyPolicySpec](#privacypolicyspec)
- [ProjectGrantMemberSpec](#projectgrantmemberspec)
- [ProjectGrantSpec](#projectgrantspec)
- [ProjectMemberSpec](#projectmemberspec)
- [ProjectRoleSpec](#projectrolespec)
- [ProjectSpec](#projectspec)
- [SAMLAppSpec](#samlappspec)
- [UserGrantSpec](#usergrantspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the referenced resource. |  |  |
| `namespace` _string_ | Namespace is the namespace of the referenced resource.<br />If empty, defaults to the same namespace as the referencing resource. |  | Optional: \{\} <br /> |


#### SAMLApp



SAMLApp is the Schema for the samlapps API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `SAMLApp` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SAMLAppSpec](#samlappspec)_ |  |  |  |
| `status` _[SAMLAppStatus](#samlappstatus)_ |  |  |  |


#### SAMLAppSpec



SAMLAppSpec defines the desired state of SAMLApp.



_Appears in:_
- [SAMLApp](#samlapp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `name` _string_ | Name is the display name of the application in Zitadel.<br />If empty, the Kubernetes resource name is used. |  | Optional: \{\} <br /> |
| `metadataXml` _string_ | MetadataXml is the SAML SP metadata XML.<br />Mutually exclusive with MetadataUrl. |  | Optional: \{\} <br /> |
| `metadataUrl` _string_ | MetadataUrl is the URL where the SAML SP metadata can be fetched.<br />Mutually exclusive with MetadataXml. |  | Optional: \{\} <br /> |


#### SAMLAppStatus



SAMLAppStatus defines the observed state of SAMLApp.



_Appears in:_
- [SAMLApp](#samlapp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applicationId` _string_ | ApplicationId is the Zitadel application ID. |  |  |
| `projectId` _string_ | ProjectId is the resolved project ID this app belongs to. |  |  |
| `organizationId` _string_ | OrganizationId is the resolved organization ID (inherited from project). |  |  |
| `ready` _boolean_ | Ready indicates whether the SAMLApp is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### ScopeMap



ScopeMap maps tenant namespaces to Zitadel org/project scopes.
Namespaced; only maps in the operator's own namespace are evaluated.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `ScopeMap` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ScopeMapSpec](#scopemapspec)_ |  |  |  |
| `status` _[ScopeMapStatus](#scopemapstatus)_ |  |  |  |


#### ScopeMapRule



ScopeMapRule maps a set of tenant namespaces to a Zitadel scope.
Exactly one of NamespaceSelector or Namespaces must be set.



_Appears in:_
- [ScopeMapSpec](#scopemapspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name identifies the rule for status reporting and Events. |  |  |
| `namespaceSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#labelselector-v1-meta)_ | NamespaceSelector selects namespaces by label.<br />Mutually exclusive with Namespaces. |  | Optional: \{\} <br /> |
| `namespaces` _string array_ | Namespaces is a literal list of namespace names.<br />Mutually exclusive with NamespaceSelector. |  | Optional: \{\} <br /> |
| `project` _string_ | Project is the Zitadel project name this rule scopes to. The project<br />is created on first use when it does not exist. When both Project and<br />ProjectId are empty, the rule grants org-scope.<br />Names are for humans, IDs for machines: when ProjectId is set it is<br />authoritative and Project is informational. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId pins the Zitadel project by raw ID (authoritative when set;<br />the project must already exist). May be used with or without a<br />Project name. |  | Optional: \{\} <br /> |


#### ScopeMapSpec



ScopeMapSpec defines the desired state of ScopeMap.



_Appears in:_
- [ScopeMap](#scopemap)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance is the operator instance identity this map applies to — the<br />operator config's instanceAlias, defaulting to its domain. Must match<br />the serving operator's identity; otherwise the map is fail-closed with<br />an InstanceMismatch condition. |  |  |
| `organization` _string_ | Organization is the Zitadel organization name. Required when<br />OrganizationId is empty (the map controller resolves the name to an<br />ID); optional and informational when OrganizationId is set. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId pins the Zitadel organization by raw ID.<br />When set, the ID is authoritative; a differing Organization name is<br />reported as drift (OrganizationNameDrift condition + Event, not an<br />error). At least one of Organization / OrganizationId must be set. |  | Optional: \{\} <br /> |
| `rules` _[ScopeMapRule](#scopemaprule) array_ | Rules is the ordered rule list. First match top-down wins<br />(evaluated across all maps in the operator namespace). |  | MinItems: 1 <br /> |


#### ScopeMapStatus



ScopeMapStatus defines the observed state of ScopeMap.



_Appears in:_
- [ScopeMap](#scopemap)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ready` _boolean_ | Ready indicates the map passed validation (instance match, org resolved). |  |  |
| `resolvedOrganizationId` _string_ | ResolvedOrganizationId is the org ID resolved from spec<br />(spec.organizationId when set, otherwise looked up by name). |  |  |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation reconciled. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the map's state. |  |  |


#### SecretKeyRef



SecretKeyRef references a key within a Kubernetes Secret.



_Appears in:_
- [GitHubIdPSpec](#githubidpspec)
- [GoogleIdPSpec](#googleidpspec)
- [HumanUserSpec](#humanuserspec)
- [SmtpEmailProvider](#smtpemailprovider)
- [TwilioSmsProvider](#twiliosmsprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Secret. |  |  |
| `key` _string_ | Key is the key within the Secret's data. Default: "clientSecret" |  | Optional: \{\} <br /> |


#### SecretKeys



SecretKeys customizes the key names used in the generated Kubernetes Secret.



_Appears in:_
- [SecretRefSpec](#secretrefspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clientId` _string_ | ClientId is the key name for the client ID in the generated secret.<br />Default: "client_id" |  | Optional: \{\} <br /> |
| `clientSecret` _string_ | ClientSecret is the key name for the client secret in the generated secret.<br />Default: "client_secret" |  | Optional: \{\} <br /> |


#### SecretRefSpec



SecretRefSpec references a Kubernetes Secret where credentials will be stored.



_Appears in:_
- [APIAppSpec](#apiappspec)
- [OIDCAppSpec](#oidcappspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Secret. |  |  |
| `keys` _[SecretKeys](#secretkeys)_ | Keys customizes the secret key names for clientId and clientSecret.<br />Defaults: clientId → "client_id", clientSecret → "client_secret" |  | Optional: \{\} <br /> |
| `extraData` _object (keys:string, values:string)_ | ExtraData adds static key-value pairs to the generated secret. |  | Optional: \{\} <br /> |


#### SmsProvider



SmsProvider is the Schema for the smsproviders API.
It manages an instance-level SMS provider (Admin API).
Discriminated type: twilio or http.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `SmsProvider` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SmsProviderSpec](#smsproviderspec)_ |  |  |  |
| `status` _[SmsProviderStatus](#smsproviderstatus)_ |  |  |  |


#### SmsProviderSpec



SmsProviderSpec defines the desired state of SmsProvider.
Discriminated type: exactly one of Twilio or Http must be set.



_Appears in:_
- [SmsProvider](#smsprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `twilio` _[TwilioSmsProvider](#twiliosmsprovider)_ | Twilio configures a Twilio SMS provider.<br />Mutually exclusive with Http. |  | Optional: \{\} <br /> |
| `http` _[HttpSmsProvider](#httpsmsprovider)_ | Http configures an HTTP webhook SMS provider.<br />Mutually exclusive with Twilio. |  | Optional: \{\} <br /> |


#### SmsProviderStatus



SmsProviderStatus defines the observed state of SmsProvider.



_Appears in:_
- [SmsProvider](#smsprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `providerId` _string_ | ProviderId is the Zitadel SMS provider ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the SmsProvider is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


#### SmtpEmailProvider



SmtpEmailProvider defines SMTP configuration for email delivery.



_Appears in:_
- [EmailProviderSpec](#emailproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ | Description is a human-readable description for this SMTP provider. |  | Optional: \{\} <br /> |
| `senderAddress` _string_ | SenderAddress is the email address used as the sender (From header). |  |  |
| `senderName` _string_ | SenderName is the display name of the sender. |  | Optional: \{\} <br /> |
| `replyToAddress` _string_ | ReplyToAddress is the reply-to email address. |  | Optional: \{\} <br /> |
| `tls` _boolean_ | Tls enables TLS for the SMTP connection. |  | Optional: \{\} <br /> |
| `host` _string_ | Host is the SMTP server hostname. |  |  |
| `user` _string_ | User is the SMTP authentication username. |  | Optional: \{\} <br /> |
| `passwordSecretRef` _[SecretKeyRef](#secretkeyref)_ | PasswordSecretRef references the Secret containing the SMTP password. |  | Optional: \{\} <br /> |


#### TokenSecretRefSpec



TokenSecretRefSpec references a Secret where the personal access token will be stored.



_Appears in:_
- [PersonalAccessTokenSpec](#personalaccesstokenspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Secret. |  |  |
| `key` _string_ | Key is the data key within the Secret. Default: "token" |  | Optional: \{\} <br /> |


#### TwilioSmsProvider



TwilioSmsProvider defines Twilio SMS configuration.



_Appears in:_
- [SmsProviderSpec](#smsproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sid` _string_ | SID is the Twilio account SID. |  |  |
| `tokenSecretRef` _[SecretKeyRef](#secretkeyref)_ | TokenSecretRef references the Secret containing the Twilio auth token. |  |  |
| `senderNumber` _string_ | SenderNumber is the Twilio phone number to send from. |  |  |
| `description` _string_ | Description is a human-readable description. |  | Optional: \{\} <br /> |


#### UserGrant



UserGrant is the Schema for the usergrants API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `zitadel.truvity.io/v1alpha2` | | |
| `kind` _string_ | `UserGrant` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[UserGrantSpec](#usergrantspec)_ |  |  |  |
| `status` _[UserGrantStatus](#usergrantstatus)_ |  |  |  |


#### UserGrantSpec



UserGrantSpec defines the desired state of UserGrant.



_Appears in:_
- [UserGrant](#usergrant)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instance` _string_ | Instance optionally pins this resource to one operator's instance<br />identity — the operator config's instanceAlias, defaulting to its<br />domain (v0.18 dual-serving). When set to another identity, the CR is<br />ignored entirely so the owning operator can manage it. When empty<br />while the namespace is served by two operators, both fail closed with<br />an AmbiguousInstance condition. |  | Optional: \{\} <br /> |
| `organizationRef` _[ResourceRef](#resourceref)_ | OrganizationRef references an Organization CR managed by this operator.<br />Mutually exclusive with OrganizationId. |  | Optional: \{\} <br /> |
| `organizationId` _string_ | OrganizationId references a pre-existing Zitadel organization by raw ID.<br />Mutually exclusive with OrganizationRef. |  | Optional: \{\} <br /> |
| `userId` _string_ | UserID is the Zitadel user ID to grant roles to. |  | Optional: \{\} <br /> |
| `userRef` _[ResourceRef](#resourceref)_ | UserRef references a MachineUser CR managed by this operator.<br />Mutually exclusive with UserID. |  | Optional: \{\} <br /> |
| `projectRef` _[ResourceRef](#resourceref)_ | ProjectRef references a Project CR managed by this operator.<br />Mutually exclusive with ProjectId. |  | Optional: \{\} <br /> |
| `projectId` _string_ | ProjectId references a pre-existing Zitadel project by raw ID.<br />Mutually exclusive with ProjectRef. |  | Optional: \{\} <br /> |
| `roleKeys` _string array_ | RoleKeys is the list of role keys to grant. |  |  |


#### UserGrantStatus



UserGrantStatus defines the observed state of UserGrant.



_Appears in:_
- [UserGrant](#usergrant)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `grantId` _string_ | GrantId is the Zitadel user grant ID. |  |  |
| `ready` _boolean_ | Ready indicates whether the UserGrant is successfully synced. |  |  |
| `lastSyncTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#time-v1-meta)_ | LastSyncTime is the last time the resource was synced with Zitadel. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state. |  |  |


