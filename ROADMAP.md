# Zitadel Operator Roadmap

## Implemented (v0.4.0)

- Organization (cluster-scoped)
- Project (cluster-scoped) with roles
- OIDCApp (namespaced) with full OIDC config
- MachineUser (namespaced) with key management
- IdentityProvider (cluster-scoped)
- LoginPolicy (cluster-scoped)
- ProjectGrant (namespaced)
- UserGrant (namespaced)
- ProjectMember (namespaced)
- ApplicationKey (namespaced)
- PersonalAccessToken (namespaced)
- PasswordComplexityPolicy (cluster-scoped)
- LockoutPolicy (cluster-scoped)

## Planned (Backlog)

- HumanUser — manage human users declaratively
- ApplicationApi — API application type (non-OIDC)
- ApplicationSaml — SAML application type
- Domain — organization domain management
- Action / TriggerActions — custom JavaScript actions on auth events
- LabelPolicy — branding and theming
- PrivacyPolicy — privacy links and ToS
- DomainPolicy — domain validation rules
- NotificationPolicy — notification settings
- SmtpConfig — email delivery configuration
- SmsProviderTwilio — SMS delivery configuration
- DefaultOidcSettings — instance-level OIDC settings (token lifetimes)
- OrgIdpOidc — organization-level generic OIDC identity provider
- IdpAzureAd — Azure AD identity provider
- IdpLdap — LDAP identity provider
- ProjectGrantMember — project grant member management
- InstanceMember — instance-level membership
- OrgMember — organization-level membership
