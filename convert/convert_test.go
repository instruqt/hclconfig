package convert

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.instruqt.com/hclconfig/test_fixtures/structs"
)

func TestTransformLabeledBlocks(t *testing.T) {
	// Create a simple CloudAccount with labeled users
	account := &structs.CloudAccount{}
	account.Meta.ID = "resource.cloud_account.test"
	account.Meta.Type = "cloud_account"
	account.Meta.Name = "test"
	account.Provider = "aws"
	account.Users = []structs.CloudUser{
		{
			Name:  "admin",
			Email: "admin@example.com",
			Roles: []string{"admin"},
		},
		{
			Name:  "developer",
			Email: "dev@example.com",
			Roles: []string{"developer"},
		},
	}
	account.Tags = map[string]string{
		"env": "test",
	}

	// Convert to cty
	ctyVal, err := GoToCtyValue(account)
	require.NoError(t, err)
	require.NotNil(t, ctyVal)

	// Check that it's an object
	require.True(t, ctyVal.Type().IsObjectType())

	valueMap := ctyVal.AsValueMap()

	// Check that user is now a map (not a list)
	userVal, exists := valueMap["user"]
	require.True(t, exists)
	require.True(t, userVal.Type().IsObjectType(), "user should be transformed to a map")

	userMap := userVal.AsValueMap()

	// Check we can access by name
	adminVal, exists := userMap["admin"]
	require.True(t, exists)
	require.True(t, adminVal.Type().IsObjectType())

	adminMap := adminVal.AsValueMap()
	emailVal := adminMap["email"]
	require.Equal(t, "admin@example.com", emailVal.AsString())

	// Check developer too
	devVal, exists := userMap["developer"]
	require.True(t, exists)
	devMap := devVal.AsValueMap()
	require.Equal(t, "dev@example.com", devMap["email"].AsString())
}

func TestLabeledBlockCtyStructure(t *testing.T) {
	// Test to understand the CTY structure of labeled blocks
	account := &structs.CloudAccount{}
	account.Meta.ID = "resource.cloud_account.test"
	account.Meta.Type = "cloud_account"
	account.Meta.Name = "test"
	account.Provider = "aws"
	account.Users = []structs.CloudUser{
		{
			Name:      "admin",
			Email:     "admin@example.com",
			Roles:     []string{"admin"},
			IamPolicy: "arn:aws:iam::aws:policy/AdministratorAccess",
		},
	}

	// Convert to cty
	ctyVal, err := GoToCtyValue(account)
	require.NoError(t, err)

	valueMap := ctyVal.AsValueMap()
	userVal := valueMap["user"]
	userMap := userVal.AsValueMap()
	adminVal := userMap["admin"]
	adminMap := adminVal.AsValueMap()

	// Log all keys in the admin user CTY map
	t.Logf("Admin user CTY map keys:")
	for k, v := range adminMap {
		t.Logf("  %q: %s (%v)", k, v.Type().FriendlyName(), v.GoString())
	}

	// Check if "name" key exists (from json tag)
	_, hasName := adminMap["name"]
	_, hasNameCap := adminMap["Name"]
	t.Logf("Has 'name' key: %v", hasName)
	t.Logf("Has 'Name' key: %v", hasNameCap)

	// Check if "meta" key exists - this is what ResourceBase needs
	_, hasMeta := adminMap["meta"]
	t.Logf("Has 'meta' key: %v", hasMeta)

	// Check if "disabled" or "depends_on" exist (ResourceBase fields)
	_, hasDisabled := adminMap["disabled"]
	_, hasDependsOn := adminMap["depends_on"]
	t.Logf("Has 'disabled' key: %v", hasDisabled)
	t.Logf("Has 'depends_on' key: %v", hasDependsOn)
}
