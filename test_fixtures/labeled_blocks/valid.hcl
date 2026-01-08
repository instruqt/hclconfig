resource "cloud_account" "production" {
  provider = "aws"

  // Named blocks - accessible by label "admin", "developer", "auditor"
  user "admin" {
    email       = "admin@example.com"
    roles       = ["admin", "billing"]
    iam_policy  = "arn:aws:iam::aws:policy/AdministratorAccess"
    mfa_enabled = true
  }

  user "developer" {
    email      = "dev@example.com"
    roles      = ["developer"]
    iam_policy = "arn:aws:iam::aws:policy/PowerUserAccess"
  }

  user "auditor" {
    email      = "audit@example.com"
    roles      = ["auditor"]
    iam_policy = "arn:aws:iam::aws:policy/ReadOnlyAccess"
  }

  // Unnamed blocks - only accessible by index [0], [1]
  permission {
    resource = "s3:*"
    actions  = ["s3:GetObject", "s3:PutObject"]
    effect   = "allow"
  }

  permission {
    resource = "ec2:*"
    actions  = ["ec2:DescribeInstances"]
    effect   = "allow"
  }

  // Simple map
  tags = {
    environment = "production"
    managed_by  = "terraform"
    team        = "platform"
  }
}

resource "cloud_account" "staging" {
  provider = "aws"

  user "developer" {
    email = "staging-dev@example.com"
    roles = ["developer", "tester"]
  }

  tags = {
    environment = "staging"
  }
}

// This resource references the production account using various access patterns
resource "cloud_account" "consumer" {
  provider = "aws"

  user "service_account" {
    // Named block access - should resolve to "admin@example.com"
    email = resource.cloud_account.production.user.admin.email

    // Test numeric index access (backward compatibility)
    iam_policy = resource.cloud_account.production.user.0.iam_policy

    // Reference multiple users by name
    roles = [
      resource.cloud_account.production.user.developer.roles[0],
      resource.cloud_account.production.user.auditor.roles[0]
    ]
  }

  // Unnamed blocks can only be accessed by index (this should still work)
  permission {
    resource = resource.cloud_account.production.permission[0].resource
    actions  = resource.cloud_account.production.permission[0].actions
    effect   = "allow"
  }

  // Map access - should work with both styles
  tags = {
    source_env  = resource.cloud_account.production.tags.environment
    source_team = resource.cloud_account.production.tags["team"]
  }
}

// Output testing different access patterns
output "admin_email" {
  value = resource.cloud_account.production.user.admin.email
}

// Test backward compatibility with numeric index access
output "admin_email_by_index" {
  value = resource.cloud_account.production.user.0.email
}

output "developer_role" {
  value = resource.cloud_account.production.user.developer.roles[0]
}

output "first_permission_resource" {
  value = resource.cloud_account.production.permission[0].resource
}

// Test dot notation for numeric index on unnamed blocks
output "first_permission_resource_dot" {
  value = resource.cloud_account.production.permission.0.resource
}

// Test non-block slice access (bracket notation)
output "admin_first_role_bracket" {
  value = resource.cloud_account.production.user.admin.roles[0]
}

// Test non-block slice access (dot notation)
output "admin_first_role_dot" {
  value = resource.cloud_account.production.user.admin.roles.0
}

// Test non-block slice access by index on different element
output "admin_second_role" {
  value = resource.cloud_account.production.user.admin.roles[1]
}

output "production_env_tag" {
  value = resource.cloud_account.production.tags.environment
}

output "production_team_tag_bracket" {
  value = resource.cloud_account.production.tags["team"]
}

// Test multiple named access
output "multiple_users" {
  value = {
    admin_email = resource.cloud_account.production.user.admin.email
    dev_email   = resource.cloud_account.production.user.developer.email
  }
}

// Test that both access patterns resolve to the same value
output "mixed_access" {
  value = {
    by_name  = resource.cloud_account.production.user.admin.email
    by_index = resource.cloud_account.production.user.0.email
    same     = resource.cloud_account.production.user.admin.email == resource.cloud_account.production.user.0.email
  }
}

// Edge case 1: Labels with special characters
resource "cloud_account" "special_chars" {
  provider = "aws"

  user "user-with-hyphens" {
    email = "hyphen@example.com"
    roles = ["user"]
  }

  user "user_with_underscores" {
    email = "underscore@example.com"
    roles = ["admin"]
  }

  user "user123" {
    email = "numeric@example.com"
    roles = ["dev"]
  }

  tags = {
    test = "special_chars"
  }
}

// Edge case 2: Cross-references between labeled elements
resource "cloud_account" "cross_ref" {
  provider = "aws"

  user "employee" {
    // Reference a user from a different resource (production)
    email = resource.cloud_account.production.user.admin.email
    roles = ["employee"]
  }

  tags = {
    test = "cross_ref"
  }
}

// Edge case 3: Both access patterns in same expression
resource "cloud_account" "mixed_patterns" {
  provider = "aws"

  user "combined" {
    // Mix both access patterns referencing production resource
    // Named access
    email = resource.cloud_account.production.user.admin.email
    // Numeric index access
    iam_policy = resource.cloud_account.production.user.1.iam_policy
    // Both in same array
    roles = [
      resource.cloud_account.production.user.0.roles[0],
      resource.cloud_account.production.user.developer.roles[0]
    ]
  }

  tags = {
    test = "mixed"
  }
}

// Outputs for edge cases
output "hyphen_user_email" {
  value = resource.cloud_account.special_chars.user["user-with-hyphens"].email
}

output "underscore_user" {
  value = resource.cloud_account.special_chars.user.user_with_underscores.email
}

output "numeric_label_name" {
  value = resource.cloud_account.special_chars.user.user123.email
}

output "numeric_label_index" {
  value = resource.cloud_account.special_chars.user.2.email
}

output "cross_ref_employee" {
  value = resource.cloud_account.cross_ref.user.employee.email
}

output "mixed_combined_roles" {
  value = resource.cloud_account.mixed_patterns.user.combined.roles
}

// Verify special char access equivalence
output "special_char_equivalence" {
  value = {
    by_name  = resource.cloud_account.special_chars.user.user123.email
    by_index = resource.cloud_account.special_chars.user.2.email
    same     = resource.cloud_account.special_chars.user.user123.email == resource.cloud_account.special_chars.user.2.email
  }
}

// =============================================================================
// Test case: Referencing labeled blocks as slice elements
// This tests the scenario where members = [resource.cloud_account.prod.user.admin]
// =============================================================================

// Source cloud account with labeled user blocks for slice reference tests
resource "cloud_account" "source" {
  provider = "aws"

  user "admin" {
    email      = "admin@example.com"
    roles      = ["admin", "owner"]
    iam_policy = "arn:aws:iam::aws:policy/AdministratorAccess"
  }

  user "developer" {
    email      = "dev@example.com"
    roles      = ["developer"]
    iam_policy = "arn:aws:iam::aws:policy/PowerUserAccess"
  }

  user "viewer" {
    email = "viewer@example.com"
    roles = ["viewer"]
  }

  permission {
    resource = "s3:*"
    actions  = ["s3:GetObject"]
    effect   = "allow"
  }

  tags = {
    environment = "production"
  }
}

// Test 1: Team with member references (whole CloudUser objects)
resource "cloud_team" "engineering" {
  name        = "Engineering Team"
  description = "Core engineering team"

  // Reference multiple labeled blocks as slice elements
  // This is the key test case: members = [resource.cloud_account.source.user.admin, ...]
  members = [
    resource.cloud_account.source.user.admin,
    resource.cloud_account.source.user.developer
  ]

  // Reference a single labeled block
  lead = resource.cloud_account.source.user.admin

  // Reference specific fields from labeled blocks (this should work)
  member_emails = [
    resource.cloud_account.source.user.admin.email,
    resource.cloud_account.source.user.developer.email,
    resource.cloud_account.source.user.viewer.email
  ]

  tags = {
    type = "engineering"
  }
}

// Test 2: Team using numeric index access (backward compatibility)
resource "cloud_team" "operations" {
  name = "Operations Team"

  // Same test but using numeric indices
  members = [
    resource.cloud_account.source.user.0,
    resource.cloud_account.source.user.2
  ]

  lead = resource.cloud_account.source.user.1

  member_emails = [
    resource.cloud_account.source.user.0.email,
    resource.cloud_account.source.user.1.email
  ]

  tags = {
    type = "operations"
  }
}

// Test 3: Mixed named and numeric access
resource "cloud_team" "mixed" {
  name = "Mixed Team"

  members = [
    resource.cloud_account.source.user.admin,
    resource.cloud_account.source.user.1,
    resource.cloud_account.source.user.viewer
  ]

  member_emails = [
    resource.cloud_account.source.user.admin.email,
    resource.cloud_account.source.user.1.email
  ]

  tags = {
    type = "mixed"
  }
}

// Outputs for slice reference tests
output "engineering_lead_email" {
  value = resource.cloud_team.engineering.lead.email
}

output "engineering_first_member_email" {
  value = resource.cloud_team.engineering.members[0].email
}

output "engineering_second_member_email" {
  value = resource.cloud_team.engineering.members[1].email
}

output "operations_lead_email" {
  value = resource.cloud_team.operations.lead.email
}

output "mixed_member_emails" {
  value = resource.cloud_team.mixed.member_emails
}

// =============================================================================
// Test case: Using actual types for labeled block references
// Tests CloudCredentials with Users []CloudUser (actual type for full round-trip)
// =============================================================================

// Source cloud account for credentials tests
resource "cloud_account" "test" {
  provider = "aws"

  user "admin" {
    email      = "admin@test.com"
    roles      = ["admin"]
    iam_policy = "arn:aws:iam::aws:policy/AdministratorAccess"
  }

  user "developer" {
    email      = "dev@test.com"
    roles      = ["developer"]
    iam_policy = "arn:aws:iam::aws:policy/PowerUserAccess"
  }

  permission {
    resource = "ec2:*"
    actions  = ["ec2:DescribeInstances"]
    effect   = "allow"
  }

  tags = {
    env = "test"
  }
}

// Test: CloudCredentials with Users []CloudUser (actual type)
// This works because the types match - full round-trip!
resource "cloud_credentials" "primary" {
  name = "Primary Credentials"

  // Using actual types: this decodes the full user object
  users = [
    resource.cloud_account.test.user.admin,
    resource.cloud_account.test.user.developer
  ]

  // Account-level references
  accounts = [
    resource.cloud_account.test
  ]
}

// Test with numeric indices (backward compatibility)
resource "cloud_credentials" "secondary" {
  name = "Secondary Credentials"

  users = [
    resource.cloud_account.test.user.0,
    resource.cloud_account.test.user.1
  ]
}

// Test with mixed access patterns
resource "cloud_credentials" "mixed" {
  name = "Mixed Credentials"

  users = [
    resource.cloud_account.test.user.admin,
    resource.cloud_account.test.user.1
  ]
}

// Outputs for credentials tests
output "test_account_first_user" {
  value = resource.cloud_account.test.user.admin.email
}

output "test_account_second_user" {
  value = resource.cloud_account.test.user.developer.email
}
