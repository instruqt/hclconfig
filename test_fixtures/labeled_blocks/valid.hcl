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
