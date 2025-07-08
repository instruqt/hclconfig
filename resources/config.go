package resources

import "github.com/instruqt/hclconfig/types"

const TypeConfig = "config"

type Config struct {
	types.ResourceBase `hcl:",remain"`

	Version string `hcl:"version,optional"`
}
