package instanceconfig

import "time"

// Known instance_config keys. These hold only non-secret instance settings;
// secrets (SMTP passwords, API keys, Twilio tokens) MUST stay in env.
const (
	KeyInstanceName    = "instance_name"
	KeyDefaultTimezone = "default_timezone"
	KeyAllowSignups    = "allow_signups"
	KeySupportEmail    = "support_email"
	KeyConfigured      = "configured"
)

// Settings is the typed view of the editable instance settings.
type Settings struct {
	InstanceName    string `json:"instanceName"`
	DefaultTimezone string `json:"defaultTimezone"`
	AllowSignups    bool   `json:"allowSignups"`
	SupportEmail    string `json:"supportEmail"`
	Configured      bool   `json:"configured"`
}

// PublicConfig is the non-sensitive subset exposed to the SPA for unauthenticated
// or general use. It deliberately omits the "configured" flag detail beyond what
// the status endpoint already reports.
type PublicConfig struct {
	InstanceName    string `json:"instanceName"`
	DefaultTimezone string `json:"defaultTimezone"`
	AllowSignups    bool   `json:"allowSignups"`
	SupportEmail    string `json:"supportEmail"`
}

// Instance is the typed singleton configuration record.
type Instance struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	DefaultTimezone  string     `json:"defaultTimezone"`
	AllowSignups     bool       `json:"allowSignups"`
	SupportEmail     string     `json:"supportEmail"`
	SetupCompletedAt *time.Time `json:"setupCompletedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// BootstrapRequest is the complete, one-time first-run transaction input.
type BootstrapRequest struct {
	BootstrapToken  string `json:"bootstrapToken"`
	AdminEmail      string `json:"adminEmail"`
	AdminName       string `json:"adminName"`
	InstanceName    string `json:"instanceName"`
	DefaultTimezone string `json:"defaultTimezone"`
	AllowSignups    bool   `json:"allowSignups"`
	SupportEmail    string `json:"supportEmail"`
}
