resource "cloud_account" "base" {
  provider = "aws"

  user "admin" {
    email = "admin@example.com"
    roles = ["admin"]
  }

  user "developer" {
    email = "dev@example.com"
    roles = ["developer"]
  }

  permission {
    resource = "s3:*"
    actions  = ["s3:GetObject"]
  }

  tags = {
    env = "test"
  }
}

// Test invalid labeled block references
resource "cloud_account" "invalid_named_ref" {
  provider = "aws"

  user "test" {
    // ERROR: "nonexistent" user doesn't exist
    email = resource.cloud_account.base.user.nonexistent.email
  }
}

resource "cloud_account" "invalid_index_on_named" {
  provider = "aws"

  user "test" {
    // ERROR: trying to use numeric index that doesn't exist
    email = resource.cloud_account.base.user[10].email
  }
}

resource "cloud_account" "invalid_named_on_unnamed" {
  provider = "aws"

  // ERROR: permissions don't have labels, can't access by name
  permission {
    resource = resource.cloud_account.base.permission.admin.resource
    actions  = ["s3:GetObject"]
  }
}

resource "cloud_account" "invalid_attribute" {
  provider = "aws"

  user "test" {
    // ERROR: "password" field doesn't exist on User
    email = resource.cloud_account.base.user.admin.password
  }
}

resource "cloud_account" "invalid_map_key" {
  provider = "aws"

  user "test" {
    // ERROR: "team" tag doesn't exist
    email = resource.cloud_account.base.tags.team
  }
}

resource "cloud_account" "mixed_invalid" {
  provider = "aws"

  user "test" {
    // ERROR: accessing nested property of non-existent user
    email = resource.cloud_account.base.user.ghost.email.something
  }
}

resource "cloud_account" "invalid_numeric_on_map" {
  provider = "aws"

  user "test" {
    // ERROR: maps can't be accessed by numeric index
    email = resource.cloud_account.base.tags[0]
  }
}

resource "cloud_account" "invalid_named_on_slice" {
  provider = "aws"

  user "test" {
    // ERROR: non-block slices can't be accessed by name
    email = resource.cloud_account.base.user.admin.roles.admin
  }
}

resource "cloud_account" "invalid_numeric_label" {
  provider = "aws"

  // ERROR: labels cannot be purely numeric
  user "0" {
    email = "zero@example.com"
    roles = ["user"]
  }

  user "123" {
    email = "oneTwoThree@example.com"
    roles = ["admin"]
  }
}
