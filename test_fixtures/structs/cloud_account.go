package structs

import "go.instruqt.com/hclconfig/types"

const TypeCloudAccount = "cloud_account"

// CloudAccount represents a cloud provider account configuration
// This tests labeled blocks, unlabeled blocks, and maps together
type CloudAccount struct {
	types.ResourceBase `hcl:"rm,remain"`

	Provider string `hcl:"provider" json:"provider"` // aws, azure, gcp

	// Named blocks - these have labels and should be accessible by name
	Users []CloudUser `hcl:"user,block" json:"users,omitempty"`

	// Unnamed blocks - these don't have labels, only accessible by index
	Permissions []Permission `hcl:"permission,block" json:"permissions,omitempty"`

	// Maps - should be accessible by key
	Tags map[string]string `hcl:"tags,optional" json:"tags,omitempty"`
}

// CloudUser represents a cloud user with a name label
// Note: Using hcl:"name,label" (not just hcl:",label") allows gohcl to:
// 1. Read the label from block syntax (user "admin" { })
// 2. Also accept "name" as an attribute when decoding from CTY references
type CloudUser struct {
	Name       string   `hcl:"name,label" json:"name"`
	Email      string   `hcl:"email" json:"email"`
	Roles      []string `hcl:"roles,optional" json:"roles,omitempty"`
	IamPolicy  string   `hcl:"iam_policy,optional" json:"iam_policy,omitempty"`
	MfaEnabled bool     `hcl:"mfa_enabled,optional" json:"mfa_enabled,omitempty"`
}

// Permission represents an unnamed permission block
type Permission struct {
	Resource string   `hcl:"resource" json:"resource"`
	Actions  []string `hcl:"actions" json:"actions"`
	Effect   string   `hcl:"effect,optional" json:"effect,omitempty"` // allow or deny
}
