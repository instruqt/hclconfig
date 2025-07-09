package resources

import "go.instruqt.com/hclconfig/types"

const TypeConfig = "config"

type Config struct {
	types.ResourceBase `hcl:",remain"`

	Version string `hcl:"version,optional"`
}
