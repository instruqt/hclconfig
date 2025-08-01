package resources

import "go.instruqt.com/hclconfig/types"

// DefaultResources is a collection of the default config resources
func DefaultResources() types.RegisteredTypes {
	return types.RegisteredTypes{
		"variable": &Variable{},
		"output":   &Output{},
		"local":    &Local{},
		"module":   &Module{},
		"root":     &Root{},
		"config":   &Config{},
	}
}
