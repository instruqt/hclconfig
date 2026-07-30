package resources

import (
	"github.com/hashicorp/hcl/v2"
	"go.instruqt.com/hclconfig/types"
)

const TypeVariable = "variable"

// Variable defines an input variable which can be set by a module or by the
// caller. Any of type, default and description may be omitted.
//
// Type is held unevaluated: a type constraint is a bare identifier
// (type = string, type = list(string)), so decoding it evaluates the identifier
// and fails with "there is no variable named string".
type Variable struct {
	types.ResourceBase `hcl:",remain"`
	Type               hcl.Expression `hcl:"type,optional" json:"-"`                            // unevaluated type constraint, if declared
	Default            any            `hcl:"default,optional" json:"default,omitempty"`         // default value for a variable
	Description        string         `hcl:"description,optional" json:"description,omitempty"` // description of the variable
}
