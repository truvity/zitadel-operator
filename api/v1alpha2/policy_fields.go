package v1alpha2

// LoginPolicyFields contains fields shared between LoginPolicy (org) and DefaultLoginPolicy (instance).
type LoginPolicyFields struct {
	UserLogin         *bool `json:"userLogin,omitempty"`
	AllowExternalIdp  *bool `json:"allowExternalIdp,omitempty"`
	AllowRegister     *bool `json:"allowRegister,omitempty"`
	ForceMfa          *bool `json:"forceMfa,omitempty"`
	ForceMfaLocalOnly *bool `json:"forceMfaLocalOnly,omitempty"`
	HidePasswordReset *bool `json:"hidePasswordReset,omitempty"`
	// +kubebuilder:validation:Enum=not_allowed;allowed
	// +optional
	PasswordlessType           string `json:"passwordlessType,omitempty"`
	AllowDomainDiscovery       *bool  `json:"allowDomainDiscovery,omitempty"`
	IgnoreUnknownUsernames     *bool  `json:"ignoreUnknownUsernames,omitempty"`
	DefaultRedirectUri         string `json:"defaultRedirectUri,omitempty"`
	PasswordCheckLifetime      string `json:"passwordCheckLifetime,omitempty"`
	ExternalLoginCheckLifetime string `json:"externalLoginCheckLifetime,omitempty"`
	MfaInitSkipLifetime        string `json:"mfaInitSkipLifetime,omitempty"`
	MultiFactorCheckLifetime   string `json:"multiFactorCheckLifetime,omitempty"`
	SecondFactorCheckLifetime  string `json:"secondFactorCheckLifetime,omitempty"`
}

// LockoutPolicyFields contains fields shared between LockoutPolicy (org) and DefaultLockoutPolicy (instance).
type LockoutPolicyFields struct {
	MaxPasswordAttempts uint32 `json:"maxPasswordAttempts"`
	// +optional
	MaxOtpAttempts uint32 `json:"maxOtpAttempts,omitempty"`
}

// PasswordComplexityPolicyFields contains fields shared between PasswordComplexityPolicy (org) and DefaultPasswordComplexityPolicy (instance).
type PasswordComplexityPolicyFields struct {
	// +kubebuilder:validation:Minimum=1
	MinLength uint64 `json:"minLength"`
	// +optional
	HasLowercase bool `json:"hasLowercase,omitempty"`
	// +optional
	HasUppercase bool `json:"hasUppercase,omitempty"`
	// +optional
	HasNumber bool `json:"hasNumber,omitempty"`
	// +optional
	HasSymbol bool `json:"hasSymbol,omitempty"`
}

// PasswordAgePolicyFields contains fields shared between PasswordAgePolicy (org) and DefaultPasswordAgePolicy (instance).
type PasswordAgePolicyFields struct {
	// MaxAgeDays is the maximum number of days a password can be used before it must be changed.
	// 0 means no expiration.
	MaxAgeDays uint32 `json:"maxAgeDays"`
	// ExpireWarnDays is the number of days before expiration to warn the user.
	// 0 means no warning.
	// +optional
	ExpireWarnDays uint32 `json:"expireWarnDays,omitempty"`
}

// NotificationPolicyFields contains fields shared between NotificationPolicy (org) and DefaultNotificationPolicy (instance).
type NotificationPolicyFields struct {
	// PasswordChange determines whether a notification is sent on password change.
	// +optional
	PasswordChange *bool `json:"passwordChange,omitempty"`
}

// PrivacyPolicyFields contains fields shared between PrivacyPolicy (org) and DefaultPrivacyPolicy (instance).
type PrivacyPolicyFields struct {
	// TosLink is the URL to the Terms of Service.
	// +optional
	TosLink string `json:"tosLink,omitempty"`
	// PrivacyLink is the URL to the Privacy Policy.
	// +optional
	PrivacyLink string `json:"privacyLink,omitempty"`
	// HelpLink is the URL to the Help/Support page.
	// +optional
	HelpLink string `json:"helpLink,omitempty"`
	// SupportEmail is the support email address.
	// +optional
	SupportEmail string `json:"supportEmail,omitempty"`
	// DocsLink is the URL to the documentation.
	// +optional
	DocsLink string `json:"docsLink,omitempty"`
	// CustomLink is a custom link URL.
	// +optional
	CustomLink string `json:"customLink,omitempty"`
	// CustomLinkText is the display text for the custom link.
	// +optional
	CustomLinkText string `json:"customLinkText,omitempty"`
}

// OIDCSettingsFields contains fields for DefaultOIDCSettings.
type OIDCSettingsFields struct {
	// AccessTokenLifetime is the duration for access token validity (e.g., "12h").
	// +optional
	AccessTokenLifetime string `json:"accessTokenLifetime,omitempty"`
	// IdTokenLifetime is the duration for ID token validity (e.g., "12h").
	// +optional
	IdTokenLifetime string `json:"idTokenLifetime,omitempty"`
	// RefreshTokenIdleExpiration is the idle expiration for refresh tokens (e.g., "720h").
	// +optional
	RefreshTokenIdleExpiration string `json:"refreshTokenIdleExpiration,omitempty"`
	// RefreshTokenExpiration is the absolute expiration for refresh tokens (e.g., "2160h").
	// +optional
	RefreshTokenExpiration string `json:"refreshTokenExpiration,omitempty"`
}

// LabelPolicyFields contains fields shared between LabelPolicy (org) and DefaultLabelPolicy (instance).
type LabelPolicyFields struct {
	// PrimaryColor is the primary brand color (hex, e.g. "#5469d4").
	// +optional
	PrimaryColor string `json:"primaryColor,omitempty"`
	// BackgroundColor is the background color (hex).
	// +optional
	BackgroundColor string `json:"backgroundColor,omitempty"`
	// WarnColor is the warning color (hex).
	// +optional
	WarnColor string `json:"warnColor,omitempty"`
	// FontColor is the font color (hex).
	// +optional
	FontColor string `json:"fontColor,omitempty"`
	// PrimaryColorDark is the primary color for dark mode (hex).
	// +optional
	PrimaryColorDark string `json:"primaryColorDark,omitempty"`
	// BackgroundColorDark is the background color for dark mode (hex).
	// +optional
	BackgroundColorDark string `json:"backgroundColorDark,omitempty"`
	// WarnColorDark is the warning color for dark mode (hex).
	// +optional
	WarnColorDark string `json:"warnColorDark,omitempty"`
	// FontColorDark is the font color for dark mode (hex).
	// +optional
	FontColorDark string `json:"fontColorDark,omitempty"`
	// HideLoginNameSuffix hides the @domain suffix on the login screen.
	// +optional
	HideLoginNameSuffix bool `json:"hideLoginNameSuffix,omitempty"`
	// DisableWatermark disables the Zitadel watermark on login pages.
	// +optional
	DisableWatermark bool `json:"disableWatermark,omitempty"`
}

// MessageTextFields contains the text fields shared by all message text types.
type MessageTextFields struct {
	// Type is the message type.
	// +kubebuilder:validation:Enum=init;passwordReset;verifyEmail;verifyPhone;verifySmsOtp;verifyEmailOtp;domainClaimed;passwordlessRegistration;passwordChange;inviteUser
	Type string `json:"type"`

	// Language is the BCP 47 language tag (e.g., "en", "de", "fr").
	Language string `json:"language"`

	// Title is the message title.
	// +optional
	Title string `json:"title,omitempty"`

	// PreHeader is the email pre-header text.
	// +optional
	PreHeader string `json:"preHeader,omitempty"`

	// Subject is the email subject line.
	// +optional
	Subject string `json:"subject,omitempty"`

	// Greeting is the greeting line (supports {{.FirstName}} template).
	// +optional
	Greeting string `json:"greeting,omitempty"`

	// Text is the main body text (supports templates).
	// +optional
	Text string `json:"text,omitempty"`

	// ButtonText is the CTA button text.
	// +optional
	ButtonText string `json:"buttonText,omitempty"`

	// FooterText is the email footer text.
	// +optional
	FooterText string `json:"footerText,omitempty"`
}
