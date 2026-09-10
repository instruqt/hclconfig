package hclconfig

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsModuleAddress(t *testing.T) {
	addresses := []string{"norncorp/first", "acme/postgres"}
	for _, source := range addresses {
		assert.True(t, isModuleAddress(source), source)
	}

	// everything go-getter understands, plus the shapes that are neither
	locations := []string{
		"github.com/instruqt/hclconfig/test_fixtures//single",
		"github.com/instruqt/hclconfig",
		"https://example.com/module.zip",
		"./single",
		"../single",
		"single",
		"norncorp/first/extra",
	}
	for _, source := range locations {
		assert.False(t, isModuleAddress(source), source)
	}
}

// A module named by address is fetched by the configured getter, which is told
// what the author asked for rather than a resolution of it.
func TestParseModuleByAddressUsesTheGetter(t *testing.T) {
	fetched, err := filepath.Abs("./test_fixtures/module_address/fetched")
	require.NoError(t, err)

	var gotSource, gotVersion string

	options := DefaultOptions()
	options.ModuleGetter = func(source, version string) (string, error) {
		gotSource, gotVersion = source, version
		return fetched, nil
	}

	p := setupParser(t, options)

	labPath, err := filepath.Abs("./test_fixtures/module_address/lab.hcl")
	require.NoError(t, err)

	c, err := p.ParseFile(labPath)
	require.NoError(t, err)

	assert.Equal(t, "norncorp/first", gotSource)
	assert.Equal(t, "~> 0.0", gotVersion)

	_, err = c.FindResource("module.first.resource.container.postgres")
	require.NoError(t, err)
}

// The parser cannot reach an address itself, so saying so beats letting
// go-getter read it as a path that does not exist.
func TestParseModuleByAddressWithoutAGetterFails(t *testing.T) {
	p := setupParser(t)

	labPath, err := filepath.Abs("./test_fixtures/module_address/lab.hcl")
	require.NoError(t, err)

	_, err = p.ParseFile(labPath)

	require.Error(t, err)
	// the rendered error wraps, so match a fragment that survives it
	assert.Contains(t, err.Error(), "is named by address")
}

// A getter that cannot supply the module fails the parse with its own reason.
func TestParseModuleByAddressReportsGetterFailure(t *testing.T) {
	options := DefaultOptions()
	options.ModuleGetter = func(source, version string) (string, error) {
		return "", fmt.Errorf("version %q was revoked", version)
	}

	p := setupParser(t, options)

	labPath, err := filepath.Abs("./test_fixtures/module_address/lab.hcl")
	require.NoError(t, err)

	_, err = p.ParseFile(labPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `version "~> 0.0" was revoked`)
}
