package hclconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.instruqt.com/hclconfig/resources"
)

func parseVariableBlock(t *testing.T, hcl string) (*Config, error) {
	t.Helper()

	file := CreateTestFile(t, hcl)
	p := NewParser(nil)

	return p.ParseFile(file)
}

// A variable block may declare any combination of type, default and
// description. Only the label is required.
func TestParseVariableAcceptsEveryAttributeCombination(t *testing.T) {
	tests := []struct {
		name string
		hcl  string
	}{
		{
			name: "type and description, no default",
			hcl: `
variable "v" {
  type        = string
  description = "a variable"
}`,
		},
		{
			name: "description only",
			hcl: `
variable "v" {
  description = "a variable"
}`,
		},
		{
			name: "type and default",
			hcl: `
variable "v" {
  type    = string
  default = "value"
}`,
		},
		{
			name: "default only",
			hcl: `
variable "v" {
  default = "value"
}`,
		},
		{
			name: "no attributes",
			hcl: `
variable "v" {
}`,
		},
		{
			name: "complex type constraint",
			hcl: `
variable "v" {
  type = list(string)
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parseVariableBlock(t, tt.hcl)
			require.NoError(t, err)

			vars, err := c.FindResourcesByType(resources.TypeVariable)
			require.NoError(t, err)
			require.Len(t, vars, 1)
		})
	}
}

// A declared default seeds the evaluation context so other resources can
// reference the variable without the caller supplying a value.
func TestParseVariableDefaultIsResolvable(t *testing.T) {
	c, err := parseVariableBlock(t, `
variable "v" {
  type    = string
  default = "from_default"
}

output "o" {
  value = variable.v
}`)
	require.NoError(t, err)

	outputs, err := c.FindResourcesByType(resources.TypeOutput)
	require.NoError(t, err)
	require.Len(t, outputs, 1)

	out := outputs[0].(*resources.Output)
	require.Equal(t, "from_default", out.Value)
}

// A variable declared without a default must still parse; it simply contributes
// no value to the context. Regression test: this previously panicked on an
// unchecked *hcl.Attribute assertion once default became optional.
func TestParseVariableWithoutDefaultDoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		_, err := parseVariableBlock(t, `
variable "v" {
  type = string
}`)
		require.NoError(t, err)
	})
}

// The reserved instruqt_* variables the lab editor writes into variables.hcl.
// This is the exact shape that reached a sandbox and failed to parse, so it
// stands in as the contract between the editor's writer and this parser.
func TestParseVariableAcceptsEditorGeneratedReservedVariables(t *testing.T) {
	c, err := parseVariableBlock(t, `
variable "instruqt_session_id" {
  type        = string
  description = "Reserved: unique ID for this lab session, provided by the platform"
}

variable "instruqt_team_id" {
  type        = string
  description = "Reserved: ID of the team running this session, provided by the platform"
}

variable "instruqt_sandbox_domain" {
  type        = string
  description = "Reserved: base domain used to construct public URLs for this sandbox, provided by the platform"
}`)
	require.NoError(t, err)

	vars, err := c.FindResourcesByType(resources.TypeVariable)
	require.NoError(t, err)
	require.Len(t, vars, 3)

	v := vars[0].(*resources.Variable)
	require.Equal(t, "Reserved: unique ID for this lab session, provided by the platform", v.Description)
	require.NotNil(t, v.Type)
}

// A genuine schema violation must still be reported, with the underlying
// diagnostic rather than a formatting artefact.
func TestParseVariableReportsRealDiagnostic(t *testing.T) {
	_, err := parseVariableBlock(t, `
variable "v" {
  not_a_real_attribute = true
}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not_a_real_attribute")
	require.NotContains(t, err.Error(), "%!s(<nil>)")
}
