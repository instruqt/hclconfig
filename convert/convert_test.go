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
