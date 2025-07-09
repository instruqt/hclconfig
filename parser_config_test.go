package hclconfig

import (
	"testing"

	"go.instruqt.com/hclconfig/resources"
	"github.com/stretchr/testify/require"
)

func TestParseConfigBlockWithValidConfig(t *testing.T) {
	hcl := `
config {
  version = "1.0.0"
}`

	file := CreateTestFile(t, hcl)
	p := NewParser(nil)

	c, err := p.ParseFile(file)
	require.NoError(t, err)
	require.NotNil(t, c)

	configs, err := c.FindResourcesByType(resources.TypeConfig)
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0].(*resources.Config)
	require.Equal(t, "1.0.0", cfg.Version)
}

func TestParseConfigBlockWithLabelReturnsError(t *testing.T) {
	hcl := `
config "invalid" {
  version = "1.0.0"
}`

	file := CreateTestFile(t, hcl)
	p := NewParser(nil)

	_, err := p.ParseFile(file)
	require.Error(t, err)
}

func TestParseConfigBlockWithEmptyConfig(t *testing.T) {
	hcl := `
config {
}`

	file := CreateTestFile(t, hcl)
	p := NewParser(nil)

	c, err := p.ParseFile(file)
	require.NoError(t, err)
	require.NotNil(t, c)

	configs, err := c.FindResourcesByType(resources.TypeConfig)
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0].(*resources.Config)
	require.Equal(t, "", cfg.Version)
}
