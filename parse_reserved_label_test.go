package hclconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.instruqt.com/hclconfig/test_fixtures/structs"
)

func writeHCL(t *testing.T, dir, name, contents string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))

	return path
}

// A resource is only ever read as resource.TYPE.NAME, so a name that reads like
// a keyword is a name like any other.
func TestParseResourceNamedAfterAKeyword(t *testing.T) {
	p := setupParser(t)

	path := writeHCL(t, t.TempDir(), "lab.hcl", `
resource "container" "module" {
  command = ["sleep", "infinity"]
}

output "target" {
  value = resource.container.module.command
}
`)

	c, err := p.ParseFile(path)
	require.NoError(t, err)

	r, err := c.FindResource("resource.container.module")
	require.NoError(t, err)
	require.Equal(t, "module", r.Metadata().Name)
}

// A module's name is part of the module path of every id below it, which is the
// case with the most reason to be refused. It is not: the ids it produces read
// back the same as any other module's.
func TestParseModuleNamedAfterAKeyword(t *testing.T) {
	p := setupParser(t)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "child"), 0o755))
	writeHCL(t, filepath.Join(dir, "child"), "main.hcl", `
resource "container" "app" {
  command = ["sleep", "infinity"]
}
`)

	path := writeHCL(t, dir, "lab.hcl", `
module "module" {
  source = "./child"
}
`)

	c, err := p.ParseFile(path)
	require.NoError(t, err)

	r, err := c.FindResource("module.module.resource.container.app")
	require.NoError(t, err)
	assert.Equal(t, "module", r.Metadata().Module)
}

// A variable is named by its one label. Reading that label without checking it
// is there crashed the parser — on a block an author is part way through
// writing, which is when they most need to be told what is missing.
func TestParseVariableWithoutAName(t *testing.T) {
	for name, lab := range map[string]string{
		"no label":    "variable {\n  default = 1\n}\n",
		"empty label": "variable \"\" {\n  default = 1\n}\n",
		"two labels":  "variable \"a\" \"b\" {\n  default = 1\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := setupParser(t)
			path := writeHCL(t, t.TempDir(), "lab.hcl", lab)

			_, err := p.ParseFile(path)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "variables should have a name")
		})
	}
}

// A variable's name is held to the same characters as any other label.
func TestParseVariableWithAnInvalidName(t *testing.T) {
	p := setupParser(t)
	path := writeHCL(t, t.TempDir(), "lab.hcl", "variable \"my*var\" {\n  default = 1\n}\n")

	_, err := p.ParseFile(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "can only contain the characters")
}

// An invalid label is refused with the reason, not an empty message: the reason
// is the only thing that tells an author what to change.
func TestParseInvalidResourceLabelReportsWhy(t *testing.T) {
	p := setupParser(t)
	p.RegisterType("container", &structs.Container{})

	path := writeHCL(t, t.TempDir(), "lab.hcl", `
resource "container" "my*container" {
  command = ["sleep", "infinity"]
}
`)

	_, err := p.ParseFile(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "can only contain the characters")
}
