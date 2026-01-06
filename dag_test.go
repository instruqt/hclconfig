package hclconfig

import (
	"path/filepath"
	"testing"

	"go.instruqt.com/hclconfig/errors"
	"go.instruqt.com/hclconfig/resources"
	"go.instruqt.com/hclconfig/test_fixtures/structs"
	"github.com/stretchr/testify/require"
)

func setupGraphConfig(t *testing.T) *Config {
	absoluteFolderPath, err := filepath.Abs("./test_fixtures/simple/container.hcl")
	if err != nil {
		t.Fatal(err)
	}

	p := NewParser(DefaultOptions())
	p.RegisterType("container", &structs.Container{})
	p.RegisterType("network", &structs.Network{})
	p.RegisterType("template", &structs.Template{})

	c, err := p.ParseFile(absoluteFolderPath)
	require.NoError(t, err)

	return c
}

func TestDoYaLikeDAGAddsDependencies(t *testing.T) {
	c := setupGraphConfig(t)

	g, err := doYaLikeDAGs(c)
	require.NoError(t, err)

	network, err := c.FindResource("resource.network.onprem")
	require.NoError(t, err)

	template, err := c.FindResource("resource.template.consul_config")
	require.NoError(t, err)

	// check the dependency tree of the base container
	base, err := c.FindResource("resource.container.base")
	require.NoError(t, err)

	s, err := g.Descendents(base)
	require.NoError(t, err)

	// check the network is returned
	list := s.List()
	require.Contains(t, list, network)

	// check the dependency tree of the consul container
	consul, err := c.FindResource("resource.container.consul")
	require.NoError(t, err)

	s, err = g.Descendents(consul)
	require.NoError(t, err)

	// check the network is returned
	list = s.List()
	require.Contains(t, list, network)
	require.Contains(t, list, base)
	require.Contains(t, list, template)
}

func TestDependenciesValidNoError(t *testing.T) {
	absoluteFolderPath, err := filepath.Abs("./test_fixtures/deps/valid.hcl")
	if err != nil {
		t.Fatal(err)
	}

	p := setupParser(t)

	_, err = p.ParseFile(absoluteFolderPath)
	require.NoError(t, err)
}

func TestDependenciesInvalidError(t *testing.T) {
	absoluteFolderPath, err := filepath.Abs("./test_fixtures/deps/invalid.hcl")
	if err != nil {
		t.Fatal(err)
	}

	p := setupParser(t)

	_, err = p.ParseFile(absoluteFolderPath)
	require.Error(t, err)

	cfgErr, ok := err.(*errors.ConfigError)
	require.True(t, ok)

	require.Len(t, cfgErr.Errors, 13)
}

func TestLabeledBlockAccess(t *testing.T) {
	absolutePath, err := filepath.Abs("./test_fixtures/labeled_blocks/valid.hcl")
	require.NoError(t, err)

	p := setupParser(t)
	p.RegisterType(structs.TypeCloudAccount, &structs.CloudAccount{})

	c, err := p.ParseFile(absolutePath)
	require.NoError(t, err)

	// Verify the production account was parsed correctly
	prod, err := c.FindResource("resource.cloud_account.production")
	require.NoError(t, err)
	require.NotNil(t, prod)

	prodAccount := prod.(*structs.CloudAccount)
	require.Equal(t, "aws", prodAccount.Provider)
	require.Len(t, prodAccount.Users, 3)
	require.Len(t, prodAccount.Permissions, 2)
	require.Len(t, prodAccount.Tags, 3)

	// Verify users are in the expected order
	require.Equal(t, "admin", prodAccount.Users[0].Name)
	require.Equal(t, "admin@example.com", prodAccount.Users[0].Email)
	require.Equal(t, "developer", prodAccount.Users[1].Name)
	require.Equal(t, "auditor", prodAccount.Users[2].Name)

	// Verify the consumer account references were resolved correctly
	consumer, err := c.FindResource("resource.cloud_account.consumer")
	require.NoError(t, err)
	require.NotNil(t, consumer)

	consumerAccount := consumer.(*structs.CloudAccount)
	require.Len(t, consumerAccount.Users, 1)

	serviceUser := consumerAccount.Users[0]
	// Named block access: user.admin.email should resolve to "admin@example.com"
	require.Equal(t, "admin@example.com", serviceUser.Email)

	// Array index access: user[0].iam_policy should resolve to admin's policy
	require.Equal(t, "arn:aws:iam::aws:policy/AdministratorAccess", serviceUser.IamPolicy)

	// Multiple named references in array
	require.Len(t, serviceUser.Roles, 2)
	require.Equal(t, "developer", serviceUser.Roles[0])
	require.Equal(t, "auditor", serviceUser.Roles[1])

	// Unnamed blocks accessed by index
	require.Len(t, consumerAccount.Permissions, 1)
	require.Equal(t, "s3:*", consumerAccount.Permissions[0].Resource)
	require.Equal(t, []string{"s3:GetObject", "s3:PutObject"}, consumerAccount.Permissions[0].Actions)

	// Map access
	require.Equal(t, "production", consumerAccount.Tags["source_env"])
	require.Equal(t, "platform", consumerAccount.Tags["source_team"])
}

func TestLabeledBlockOutputs(t *testing.T) {
	absolutePath, err := filepath.Abs("./test_fixtures/labeled_blocks/valid.hcl")
	require.NoError(t, err)

	p := setupParser(t)
	p.RegisterType(structs.TypeCloudAccount, &structs.CloudAccount{})

	c, err := p.ParseFile(absolutePath)
	require.NoError(t, err)

	// Named access output
	out, err := c.FindResource("output.admin_email")
	require.NoError(t, err)
	adminEmail := out.(*resources.Output)
	require.Equal(t, "admin@example.com", adminEmail.Value)

	// Index access output (backward compatibility)
	out, err = c.FindResource("output.admin_email_by_index")
	require.NoError(t, err)
	adminEmailByIndex := out.(*resources.Output)
	require.Equal(t, "admin@example.com", adminEmailByIndex.Value)

	// Named access to different user
	out, err = c.FindResource("output.developer_role")
	require.NoError(t, err)
	devRole := out.(*resources.Output)
	require.Equal(t, "developer", devRole.Value)

	// Unnamed block by index (bracket notation)
	out, err = c.FindResource("output.first_permission_resource")
	require.NoError(t, err)
	permResource := out.(*resources.Output)
	require.Equal(t, "s3:*", permResource.Value)

	// Unnamed block by index (dot notation)
	out, err = c.FindResource("output.first_permission_resource_dot")
	require.NoError(t, err)
	permResourceDot := out.(*resources.Output)
	require.Equal(t, "s3:*", permResourceDot.Value)

	// Non-block slice access (bracket notation)
	out, err = c.FindResource("output.admin_first_role_bracket")
	require.NoError(t, err)
	adminRoleBracket := out.(*resources.Output)
	require.Equal(t, "admin", adminRoleBracket.Value)

	// Non-block slice access (dot notation)
	out, err = c.FindResource("output.admin_first_role_dot")
	require.NoError(t, err)
	adminRoleDot := out.(*resources.Output)
	require.Equal(t, "admin", adminRoleDot.Value)

	// Non-block slice access by index on different element
	out, err = c.FindResource("output.admin_second_role")
	require.NoError(t, err)
	adminSecondRole := out.(*resources.Output)
	require.Equal(t, "billing", adminSecondRole.Value)

	// Map access (dot notation)
	out, err = c.FindResource("output.production_env_tag")
	require.NoError(t, err)
	envTag := out.(*resources.Output)
	require.Equal(t, "production", envTag.Value)

	// Map access (bracket notation)
	out, err = c.FindResource("output.production_team_tag_bracket")
	require.NoError(t, err)
	teamTag := out.(*resources.Output)
	require.Equal(t, "platform", teamTag.Value)

	// Mixed access pattern
	out, err = c.FindResource("output.mixed_access")
	require.NoError(t, err)
	mixed := out.(*resources.Output)
	mixedMap := mixed.Value.(map[string]any)
	require.Equal(t, "admin@example.com", mixedMap["by_name"])
	require.Equal(t, "admin@example.com", mixedMap["by_index"])
	require.Equal(t, true, mixedMap["same"])

	// Edge case: Labels with hyphens (bracket notation required)
	out, err = c.FindResource("output.hyphen_user_email")
	require.NoError(t, err)
	hyphenEmail := out.(*resources.Output)
	require.Equal(t, "hyphen@example.com", hyphenEmail.Value)

	// Edge case: Labels with underscores (dot notation works)
	out, err = c.FindResource("output.underscore_user")
	require.NoError(t, err)
	underscoreEmail := out.(*resources.Output)
	require.Equal(t, "underscore@example.com", underscoreEmail.Value)

	// Edge case: Numeric string labels by name
	out, err = c.FindResource("output.numeric_label_name")
	require.NoError(t, err)
	numericName := out.(*resources.Output)
	require.Equal(t, "numeric@example.com", numericName.Value)

	// Edge case: Numeric string labels by index
	out, err = c.FindResource("output.numeric_label_index")
	require.NoError(t, err)
	numericIndex := out.(*resources.Output)
	require.Equal(t, "numeric@example.com", numericIndex.Value)

	// Edge case: Cross-reference (user referencing another resource's user)
	out, err = c.FindResource("output.cross_ref_employee")
	require.NoError(t, err)
	crossRefEmail := out.(*resources.Output)
	require.Equal(t, "admin@example.com", crossRefEmail.Value)

	// Edge case: Mixed patterns in same expression
	out, err = c.FindResource("output.mixed_combined_roles")
	require.NoError(t, err)
	combinedRoles := out.(*resources.Output)
	rolesSlice := combinedRoles.Value.([]any)
	require.Equal(t, 2, len(rolesSlice))
	require.Equal(t, "admin", rolesSlice[0])
	require.Equal(t, "developer", rolesSlice[1])

	// Edge case: Special char equivalence
	out, err = c.FindResource("output.special_char_equivalence")
	require.NoError(t, err)
	specialEq := out.(*resources.Output)
	specialMap := specialEq.Value.(map[string]any)
	require.Equal(t, "numeric@example.com", specialMap["by_name"])
	require.Equal(t, "numeric@example.com", specialMap["by_index"])
	require.Equal(t, true, specialMap["same"])
}

func TestLabeledBlockValidation(t *testing.T) {
	// Test that validation works correctly for labeled blocks
	absolutePath, err := filepath.Abs("./test_fixtures/labeled_blocks/valid.hcl")
	require.NoError(t, err)

	p := setupParser(t)
	p.RegisterType(structs.TypeCloudAccount, &structs.CloudAccount{})

	c, err := p.ParseFile(absolutePath)
	require.NoError(t, err)

	// Get the production resource
	prod, err := c.FindResource("resource.cloud_account.production")
	require.NoError(t, err)
	require.NotNil(t, prod)

	// Verify links were detected (these are the references in the consumer resource)
	consumer, err := c.FindResource("resource.cloud_account.consumer")
	require.NoError(t, err)
	require.NotNil(t, consumer)

	links := consumer.Metadata().Links
	require.Contains(t, links, "resource.cloud_account.production.user.admin.email")
	require.Contains(t, links, "resource.cloud_account.production.user[0].iam_policy")
	require.Contains(t, links, "resource.cloud_account.production.user.developer.roles[0]")
	require.Contains(t, links, "resource.cloud_account.production.user.auditor.roles[0]")
	require.Contains(t, links, "resource.cloud_account.production.permission[0].resource")
	require.Contains(t, links, "resource.cloud_account.production.permission[0].actions")
	require.Contains(t, links, "resource.cloud_account.production.tags.environment")
}

func TestLabeledBlockInvalidReferences(t *testing.T) {
	absolutePath, err := filepath.Abs("./test_fixtures/labeled_blocks/invalid.hcl")
	require.NoError(t, err)

	p := setupParser(t)
	p.RegisterType(structs.TypeCloudAccount, &structs.CloudAccount{})

	_, err = p.ParseFile(absolutePath)
	require.Error(t, err)

	cfgErr, ok := err.(*errors.ConfigError)
	require.True(t, ok)
	require.True(t, cfgErr.ContainsErrors())

	// We should have multiple errors for all the invalid references
	require.Greater(t, len(cfgErr.Errors), 0)

	// Check that we get meaningful error messages
	errorMessages := make([]string, len(cfgErr.Errors))
	for i, e := range cfgErr.Errors {
		errorMessages[i] = e.Error()
		t.Logf("Error %d: %s", i+1, e.Error())
	}

	// Should have errors for:
	// 1. Nonexistent named user
	require.True(t, containsSubstring(errorMessages, "nonexistent"))

	// 2. Out of bounds index
	require.True(t, containsSubstring(errorMessages, "10") || containsSubstring(errorMessages, "index"))

	// 3. Trying to use name on unnamed blocks
	require.True(t, containsSubstring(errorMessages, "admin") || containsSubstring(errorMessages, "permission"))

	// 4. Invalid attribute
	require.True(t, containsSubstring(errorMessages, "password"))

	// 5. Invalid map key
	require.True(t, containsSubstring(errorMessages, "team"))

	// 6. Nested invalid reference
	require.True(t, containsSubstring(errorMessages, "ghost"))

	// 7. Numeric index on map
	require.True(t, containsSubstring(errorMessages, "\"0\"") || containsSubstring(errorMessages, "map does not contain"))

	// 8. Named access on non-block slice
	require.True(t, containsSubstring(errorMessages, "invalid list index") || containsSubstring(errorMessages, "roles"))

	// 9. Purely numeric labels
	require.True(t, containsSubstring(errorMessages, "purely numeric") || containsSubstring(errorMessages, "\"0\"") || containsSubstring(errorMessages, "\"123\""))
}

// Helper function to check if any error message contains a substring
func containsSubstring(messages []string, substr string) bool {
	for _, msg := range messages {
		if contains(msg, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInMiddle(s, substr)))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
