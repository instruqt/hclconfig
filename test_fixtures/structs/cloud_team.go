package structs

import "go.instruqt.com/hclconfig/types"

const TypeCloudTeam = "cloud_team"

// CloudTeam represents a team configuration that references users from CloudAccount
// This tests the scenario where labeled block elements are referenced as slice items
//
// Key insight: CloudUser uses hcl:"name,label" which allows it to:
// 1. Read the label from block syntax (user "admin" { }) when parsing HCL
// 2. Accept "name" as an attribute when decoding from CTY references
//
// This eliminates the need for a separate "ref" type!
type CloudTeam struct {
	types.ResourceBase `hcl:"rm,remain"`

	Name        string `hcl:"name" json:"name"`
	Description string `hcl:"description,optional" json:"description,omitempty"`

	// Members are references to CloudUser blocks from CloudAccount resources
	// Because CloudUser uses hcl:"name,label", it can decode the "name" attribute from CTY
	// This tests: members = [resource.cloud_account.prod.user.admin, ...]
	Members []CloudUser `hcl:"members,optional" json:"members,omitempty"`

	// SingleMember tests a single reference to a labeled block
	// This tests: lead = resource.cloud_account.prod.user.admin
	Lead *CloudUser `hcl:"lead,optional" json:"lead,omitempty"`

	// MemberEmails tests extracting specific fields from referenced labeled blocks
	// This tests: member_emails = [resource.cloud_account.prod.user.admin.email]
	MemberEmails []string `hcl:"member_emails,optional" json:"member_emails,omitempty"`

	// Tags for additional metadata
	Tags map[string]string `hcl:"tags,optional" json:"tags,omitempty"`
}

// CloudCredentials represents a credential configuration
// Using actual types (CloudUser) instead of ResourceBase allows full round-trip
// and access to all user fields in code
type CloudCredentials struct {
	types.ResourceBase `hcl:"rm,remain"`

	Name string `hcl:"name" json:"name"`

	// Users are the ACTUAL user objects - full round-trip works!
	// users = [resource.aws_account.test.user.admin] decodes to CloudUser with all fields
	Users []CloudUser `hcl:"users,optional" json:"users,omitempty"`

	// Accounts references whole cloud accounts
	Accounts []CloudAccount `hcl:"accounts,optional" json:"accounts,omitempty"`
}

const TypeCloudCredentials = "cloud_credentials"
