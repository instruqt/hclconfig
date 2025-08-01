package hclconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"go.instruqt.com/hclconfig/errors"
	"go.instruqt.com/hclconfig/registry"
	"go.instruqt.com/hclconfig/resources"
	"go.instruqt.com/hclconfig/types"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

var rootContext *hcl.EvalContext

type ResourceTypeNotExistError struct {
	Type string
	File string
}

func (r ResourceTypeNotExistError) Error() string {
	return fmt.Sprintf("Resource type %s defined in file %s, is not a registered resource.", r.Type, r.File)
}

type ParserOptions struct {
	// list of default variable values to add to the parser
	Variables map[string]string
	// list of variable files to be read by the parser
	VariablesFiles []string
	// environment variable prefix
	VariableEnvPrefix string
	// location of any downloaded modules
	ModuleCache string
	// default registry to use when fetching modules
	DefaultRegistry string
	// credentials to use with the registries
	RegistryCredentials map[string]string
	// Callback executed when the parser reads a resource stanza, callbacks are
	// executed based on a directed acyclic graph. If resource 'a' references
	// a property defined in resource 'b', i.e 'resource.a.myproperty' then the
	// callback for resource 'b' will be executed before resource 'a'. This allows
	// you to set the dependent properties of resource 'b' before resource 'a'
	// consumes them.
	Callback WalkCallback

	// PrimativesOnly will parse a structure including modules:
	// * registered types for the resources are not loaded, all resources are
	//   parsed as ResourceBase, custom properties are discarded
	// * the graph of resources is not walked, any interpolated properties
	//   are not resolved.
	PrimativesOnly bool
}

// DefaultOptions returns a ParserOptions object with the
// ModuleCache set to the default directory of $HOME/.hclconfig/cache
// if the $HOME folder can not be determined, the cache is set to the
// current folder
// VariableEnvPrefix is set to 'HCL_VAR_', should a variable be defined
// called 'foo' setting the environment variable 'HCL_VAR_foo' will override
// any default value
func DefaultOptions() *ParserOptions {
	cacheDir, err := os.UserHomeDir()
	if err != nil {
		cacheDir = "."
	}

	cacheDir = filepath.Join(cacheDir, ".hclconfig", "cache")
	os.MkdirAll(cacheDir, os.ModePerm)

	registryCredentials := map[string]string{}

	return &ParserOptions{
		ModuleCache:         cacheDir,
		VariableEnvPrefix:   "HCL_VAR_",
		RegistryCredentials: registryCredentials,
	}
}

// Parser can parse HCL configuration files
type Parser struct {
	options             ParserOptions
	registeredTypes     types.RegisteredTypes
	registeredFunctions map[string]function.Function
}

// NewParser creates a new parser with the given options
// if options are nil, default options are used
func NewParser(options *ParserOptions) *Parser {
	o := options
	if o == nil {
		o = DefaultOptions()
	}

	return &Parser{options: *o, registeredTypes: resources.DefaultResources(), registeredFunctions: map[string]function.Function{}}
}

// RegisterType type registers a struct that implements Resource with the given name
// the parser uses this list to convert hcl defined resources into concrete types
func (p *Parser) RegisterType(name string, resource types.Resource) {
	p.registeredTypes[name] = resource
}

// RegisterFunction type registers a custom interpolation function
// with the given name
// the parser uses this list to convert hcl defined resources into concrete types
func (p *Parser) RegisterFunction(name string, f any) error {
	ctyFunc, err := createCtyFunctionFromGoFunc(f)
	if err != nil {
		return nil
	}

	p.registeredFunctions[name] = ctyFunc

	return nil
}

// ParseDirectory parses all resources in the given file
// error can be cast to *ConfigError to get a list of errors
func (p *Parser) ParseFile(file string) (*Config, error) {
	c := NewConfig()
	rootContext = buildContext(file, p.registeredFunctions)

	ce := errors.NewConfigError()

	err := p.parseFile(rootContext, file, c, p.options.Variables, p.options.VariablesFiles)
	if err != nil {
		for _, e := range err {
			ce.AppendError(e)
		}

		fmt.Println("parse error, skipping call Parse")
		return nil, ce
	}

	for _, rt := range c.Resources {
		// call the resources Parse function if set
		// if the config implements the processable interface call the resource process method
		if p, ok := rt.(types.Parsable); ok {
			err := p.Parse(c)
			if err != nil {
				de := &errors.ParserError{}
				de.Line = rt.Metadata().Line
				de.Column = rt.Metadata().Column
				de.Filename = rt.Metadata().File
				de.Level = errors.ParserErrorLevelError
				de.Message = fmt.Sprintf(`error parsing resource "%s" %s`, resources.FQRNFromResource(rt).String(), err)

				ce.AppendError(de)
			}
		}
	}

	if len(ce.Errors) > 0 {
		return nil, ce
	}

	// do not walk the dag when we are only dealing with primatives
	if p.options.PrimativesOnly {
		return c, nil
	}

	// process the files and resolve dependency
	return c, p.process(c)
}

// ParseDirectory parses all resource and variable files in the given directory
// note: this method does not recurse into sub folders
// error can be cast to *ConfigError to get a list of errors
func (p *Parser) ParseDirectory(dir string) (*Config, error) {
	c := NewConfig()
	rootContext = buildContext(dir, p.registeredFunctions)

	ce := errors.NewConfigError()

	err := p.parseDirectory(rootContext, dir, c)
	if err != nil {
		for _, e := range err {
			ce.AppendError(e)
		}

		return nil, ce
	}

	for _, rt := range c.Resources {
		// call the resources Parse function if set
		// if the config implements the processable interface call the resource process method
		if p, ok := rt.(types.Parsable); ok {
			err := p.Parse(c)
			if err != nil {
				de := &errors.ParserError{}
				de.Level = errors.ParserErrorLevelError
				de.Line = rt.Metadata().Line
				de.Column = rt.Metadata().Column
				de.Filename = rt.Metadata().File
				de.Message = fmt.Sprintf(`error parsing resource "%s" %s`, resources.FQRNFromResource(rt).String(), err)

				ce.AppendError(de)
			}
		}
	}

	if len(ce.Errors) > 0 {
		return nil, ce
	}

	// do not walk the dag when we are only dealing with primatives
	if p.options.PrimativesOnly {
		return c, nil
	}

	// process the files and resolve dependency
	return c, p.process(c)
}

// UnmarshalJSON parses a JSON string from a serialized Config and returns a
// valid Config.
func (p *Parser) UnmarshalJSON(d []byte) (*Config, error) {
	conf := NewConfig()

	var objMap map[string]*json.RawMessage
	err := json.Unmarshal(d, &objMap)
	if err != nil {
		return nil, err
	}

	var rawMessagesForResources []*json.RawMessage
	err = json.Unmarshal(*objMap["resources"], &rawMessagesForResources)
	if err != nil {
		return nil, err
	}

	for _, m := range rawMessagesForResources {
		mm := map[string]any{}
		err := json.Unmarshal(*m, &mm)
		if err != nil {
			return nil, err
		}

		meta := mm["meta"].(map[string]any)

		r, err := p.registeredTypes.CreateResource(meta["type"].(string), meta["name"].(string))
		if err != nil {
			return nil, err
		}

		resData, _ := json.Marshal(mm)

		json.Unmarshal(resData, r)
		conf.addResource(r, nil, nil)
	}

	return conf, nil
}

// internal method
func (p *Parser) parseDirectory(ctx *hcl.EvalContext, dir string, c *Config) []error {

	// get all files in a directory
	path, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return []error{fmt.Errorf("directory %s does not exist", dir)}
	}

	if !path.IsDir() {
		return []error{fmt.Errorf("%s is not a directory", dir)}
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return []error{fmt.Errorf("unable to list files in directory %s, error: %s", dir, err)}
	}

	variablesFiles := p.options.VariablesFiles

	// first process vars files
	for _, f := range files {
		fn := filepath.Join(dir, f.Name())

		if !f.IsDir() {
			if strings.HasSuffix(fn, ".vars") {
				// add to the collection
				variablesFiles = append(variablesFiles, fn)
			}
		}
	}

	for _, f := range files {
		fn := filepath.Join(dir, f.Name())

		if !f.IsDir() {
			if strings.HasSuffix(fn, ".hcl") {
				err := p.parseFile(ctx, fn, c, p.options.Variables, variablesFiles)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// parseFile loads variables and resources from the given file
func (p *Parser) parseFile(
	ctx *hcl.EvalContext,
	file string,
	c *Config,
	variables map[string]string,
	variablesFile []string) []error {

	// This must be done before any other process as the resources
	// might reference the variables
	err := p.parseVariablesInFile(ctx, file, c)
	if err != nil {
		return []error{err}
	}

	// override any variables from files
	for _, vf := range variablesFile {
		err := p.loadVariablesFromFile(ctx, vf)
		if err != nil {
			return []error{err}
		}
	}

	// override default values for variables from environment or variables map
	p.setVariables(ctx, variables)

	errs := p.parseResourcesInFile(ctx, file, c, "", false, []string{})
	if errs != nil {
		return errs
	}

	return nil
}

// loadVariablesFromFile loads variable values from a file
func (p *Parser) loadVariablesFromFile(ctx *hcl.EvalContext, path string) error {
	parser := hclparse.NewParser()

	f, diag := parser.ParseHCLFile(path)
	if diag.HasErrors() {
		de := &errors.ParserError{}
		de.Line = diag[0].Subject.Start.Line
		de.Column = diag[0].Subject.Start.Line
		de.Filename = path
		de.Level = errors.ParserErrorLevelError
		de.Message = fmt.Sprintf("unable to parse file: %s", diag[0].Detail)
	}

	attrs, _ := f.Body.JustAttributes()
	for name, attr := range attrs {
		val, _ := attr.Expr.Value(ctx)

		setContextVariable(ctx, name, val)
	}

	return nil
}

// setVariables allow variables to be set from a collection or environment variables
// Precedence should be file, env, vars
func (p *Parser) setVariables(ctx *hcl.EvalContext, vars map[string]string) {
	// first any vars defined as environment variables
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, p.options.VariableEnvPrefix) {
			parts := strings.Split(e, "=")

			if len(parts) == 2 {
				key := strings.Replace(parts[0], p.options.VariableEnvPrefix, "", -1)
				setContextVariable(ctx, key, valueFromString(parts[1]))
			}
		}
	}

	// then set vars
	for k, v := range vars {
		setContextVariable(ctx, k, valueFromString(v))
	}
}

func valueFromString(v string) cty.Value {
	// attempt to parse the string value into a known type
	if val, err := strconv.ParseInt(v, 10, 0); err == nil {
		return cty.NumberIntVal(val)
	}

	if val, err := strconv.ParseBool(v); err == nil {
		return cty.BoolVal(val)
	}

	// otherwise return a string
	return cty.StringVal(v)
}

// ParseVariableFile parses a config file for variables
func (p *Parser) parseVariablesInFile(ctx *hcl.EvalContext, file string, c *Config) error {
	parser := hclparse.NewParser()

	f, diag := parser.ParseHCLFile(file)
	if diag.HasErrors() {
		de := &errors.ParserError{}

		if diag[0].Subject != nil {
			de.Line = diag[0].Subject.Start.Line
			de.Column = diag[0].Subject.Start.Line
		}

		de.Level = errors.ParserErrorLevelError
		de.Filename = file
		de.Message = fmt.Sprintf("unable to parse file: %s", diag[0].Detail)

		return de
	}

	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		panic("Error getting body")
	}

	for _, b := range body.Blocks {
		switch b.Type {
		case resources.TypeVariable:
			r, _ := p.registeredTypes.CreateResource(resources.TypeVariable, b.Labels[0])
			v := r.(*resources.Variable)

			// add the checksum for the resource
			cs, err := ReadFileLocation(b.Range().Filename, b.Range().Start.Line, b.TypeRange.Start.Column, b.Range().End.Line, b.Range().End.Column)
			if err != nil {
				panic(err)
			}

			r.Metadata().Checksum.Parsed = HashString(cs)

			err = decodeBody(ctx, c, file, b, v, false)
			if err != nil {
				return err
			}

			// add the variable to the context
			c.AppendResource(v)

			val, _ := v.Default.(*hcl.Attribute).Expr.Value(ctx)
			setContextVariableIfMissing(ctx, v.Meta.Name, val)
		}
	}

	return nil
}

// parseResourcesInFile parses a hcl file and adds any found resources to the config
func (p *Parser) parseResourcesInFile(ctx *hcl.EvalContext, file string, c *Config, moduleName string, disabled bool, dependsOn []string) []error {
	parser := hclparse.NewParser()

	f, diag := parser.ParseHCLFile(file)
	if diag.HasErrors() {
		de := &errors.ParserError{}
		de.Line = diag[0].Subject.Start.Line
		de.Column = diag[0].Subject.Start.Line
		de.Filename = file
		de.Message = fmt.Sprintf("unable to parse file: %s", diag[0].Detail)

		// check the error types and determine if we should set a warning or error
		level := errors.ParserErrorLevelWarning

		for _, e := range diag.Errs() {
			err, ok := e.(*hcl.Diagnostic)
			if !ok {
				continue
			}

			if err.Summary == "Error in function call" {
				level = errors.ParserErrorLevelError
				break
			}
		}

		de.Level = level
		return []error{de}
	}

	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		// this should never happen, body should always be a hclsyntax.Body
		panic("Error getting body")
	}

	for _, b := range body.Blocks {
		// check the resource has a name
		// b.Type != resources.TypeConfig -> has no labels
		if len(b.Labels) == 0 && b.Type != resources.TypeConfig {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Message = fmt.Sprintf("resource '%s' has no name, please specify resources using the syntax 'resource_type \"name\" {}'", b.Type)

			return []error{de}
		}

		// create the registered type if not a variable or output
		// variables and outputs are processed in a separate run
		switch b.Type {
		case resources.TypeVariable:
			continue
		case resources.TypeModule:
			err := p.parseModule(ctx, c, file, b, moduleName, dependsOn)
			if err != nil {
				return err
			}
		case resources.TypeConfig:
			if len(b.Labels) > 0 {
				de := &errors.ParserError{}
				de.Line = b.TypeRange.Start.Line
				de.Column = b.TypeRange.Start.Column
				de.Filename = file
				de.Level = errors.ParserErrorLevelError
				de.Message = `config blocks should not have labels`
				return []error{de}
			}
			rt, _ := p.registeredTypes.CreateResource(resources.TypeConfig, "config")
			rt.Metadata().Module = moduleName
			rt.Metadata().File = file
			rt.Metadata().Line = b.TypeRange.Start.Line
			rt.Metadata().Column = b.TypeRange.Start.Column
			decodeBody(ctx, c, file, b, rt, false)
			c.addResource(rt, ctx, b.Body)
			continue
		case resources.TypeOutput:
			fallthrough
		case resources.TypeLocal:
			fallthrough
		case types.TypeResource:
			err := p.parseResource(ctx, c, file, b, moduleName, dependsOn, disabled)
			if err != nil {
				return []error{err}
			}
		default:
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelWarning
			de.Message = fmt.Sprintf("unable to process stanza '%s' in file %s at %d,%d , only 'variable', 'resource', 'module', 'output', and 'config' are valid stanza blocks", b.Type, file, b.Range().Start.Line, b.Range().Start.Column)

			return []error{de}
		}
	}

	return nil
}

func setDependsOn(ctx *hcl.EvalContext, r types.Resource, b *hclsyntax.Body, dependsOn []string) error {
	for _, d := range dependsOn {
		r.AddDependency(d)
	}

	if attr, ok := b.Attributes["depends_on"]; ok {
		dependsOnVal, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return fmt.Errorf("unable to read depends_on attribute: %s", diags.Error())
		}

		// depends on is a slice of string
		dependsOnSlice := dependsOnVal.AsValueSlice()
		for _, d := range dependsOnSlice {
			_, err := resources.ParseFQRN(d.AsString())
			if err != nil {
				return fmt.Errorf("invalid dependency %s, %s", d.AsString(), err)
			}

			r.AddDependency(d.AsString())
		}
	}

	return nil
}

func (p *Parser) parseModule(ctx *hcl.EvalContext, c *Config, file string, b *hclsyntax.Block, moduleName string, dependsOn []string) []error {
	// check the module has a name
	if len(b.Labels) != 1 {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = `invalid syntax for 'module' stanza, modules should be formatted 'module "name" {}`

		return []error{de}
	}

	name := b.Labels[0]
	if err := validateResourceName(name); err != nil {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = err.Error()

		return []error{de}
	}

	rt, _ := resources.DefaultResources().CreateResource(string(resources.TypeModule), b.Labels[0])

	rt.Metadata().Module = moduleName
	rt.Metadata().File = file
	rt.Metadata().Line = b.TypeRange.Start.Line
	rt.Metadata().Column = b.TypeRange.Start.Column

	err := decodeBody(ctx, c, file, b, rt, false)
	if err != nil {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = fmt.Sprintf("error creating resource: %s", err)

		return []error{de}
	}

	derr := setDependsOn(ctx, rt, b.Body, dependsOn)
	if derr != nil {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = derr.Error()

		return []error{de}
	}

	// we need to fetch the source so that we can process the child resources
	// "source" is the attribute but we need to read this manually
	if _, ok := b.Body.Attributes["source"]; !ok {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = "no source property set for module resource"

		return []error{de}
	}

	// we need to fetch the source so that we can process the child resources
	// "source" is the attribute but we need to read this manually
	src, diags := b.Body.Attributes["source"].Expr.Value(ctx)
	if diags.HasErrors() {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = fmt.Sprintf("unable to read source from module: %s", diags.Error())

		return []error{de}
	}

	version := "latest"
	if b.Body.Attributes["version"] != nil {
		v, diags := b.Body.Attributes["version"].Expr.Value(ctx)
		if diags.HasErrors() {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = fmt.Sprintf("unable to read version from module: %s", diags.Error())

			return []error{de}
		}
		version = v.AsString()
	}

	rt.(*resources.Module).Source = src.AsString()
	rt.(*resources.Module).Version = version

	// src could be a registry url, github repository or a relative folder
	// first check if it is a folder, we need to make it absolute relative to the current file
	dir := path.Dir(file)
	moduleSrc := path.Join(dir, src.AsString())

	fi, serr := os.Stat(moduleSrc)
	if serr != nil || !fi.IsDir() {
		moduleURL := src.AsString()

		parts := strings.Split(moduleURL, "/")

		// if there are 2 parts (namespace, module), check if the default registry is set
		if len(parts) == 2 && p.options.DefaultRegistry != "" {
			parts = append([]string{p.options.DefaultRegistry}, parts...)
		}

		// if there are 3 parts (registry, namespace, module) it could be a registry
		if len(parts) == 3 {
			host := parts[0]
			namespace := parts[1]
			name := parts[2]

			// check if the registry has credentials
			var token string
			if _, ok := p.options.RegistryCredentials[host]; ok {
				token = p.options.RegistryCredentials[host]
			}

			// if we can't create a registry, it is not a module registry so we can ignore the error
			r, err := registry.New(host, token)
			if err == nil {
				// get all available versions of the module from the registry
				// check if the requested version exists
				versions, err := r.GetModuleVersions(namespace, name)
				if err != nil {
					de := &errors.ParserError{}
					de.Line = b.TypeRange.Start.Line
					de.Column = b.TypeRange.Start.Column
					de.Filename = file
					de.Level = errors.ParserErrorLevelError
					de.Message = err.Error()

					return []error{de}
				}

				// if no version is set, use latest
				if version == "latest" {
					version = versions.Latest
				} else {
					// otherwise check the version exists
					versionExists := false
					for _, v := range versions.Versions {
						if v.Version == version {
							versionExists = true
							break
						}
					}

					if !versionExists {
						de := &errors.ParserError{}
						de.Line = b.TypeRange.Start.Line
						de.Column = b.TypeRange.Start.Column
						de.Filename = file
						de.Level = errors.ParserErrorLevelError
						de.Message = fmt.Sprintf(`version "%s" does not exist for module "%s/%s" in registry "%s"`, version, namespace, name, host)

						return []error{de}
					}
				}

				module, err := r.GetModule(namespace, name, version)
				if err == nil {
					// if we get back a module url from the registry,
					// set the source to the returned url
					moduleURL = module.DownloadURL
				} else {
					de := &errors.ParserError{}
					de.Line = b.TypeRange.Start.Line
					de.Column = b.TypeRange.Start.Column
					de.Filename = file
					de.Level = errors.ParserErrorLevelError
					de.Message = fmt.Sprintf(`unable to fetch module "%s/%s" from registry "%s": %s`, namespace, name, host, err)

					return []error{de}
				}
			}
		}

		// is not a directory fetch from source using go getter
		gg := NewGoGetter()

		mp, err := gg.Get(moduleURL, p.options.ModuleCache, false)
		if err != nil {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = fmt.Sprintf(`unable to fetch remote module "%s": %s`, src.AsString(), err)

			return []error{de}
		}

		moduleSrc = mp
	}

	// create a new config and add the resources later
	moduleConfig := NewConfig()

	// modules should have their own context so that variables are not globally scoped
	subContext := buildContext(moduleSrc, p.registeredFunctions)

	errs := p.parseDirectory(subContext, moduleSrc, moduleConfig)
	if errs != nil {
		return errs
	}

	rt.(*resources.Module).SubContext = subContext

	// add the module
	c.addResource(rt, ctx, b.Body)

	// we need to add the module name to all the returned resources
	for _, r := range moduleConfig.Resources {
		// ensure the module name has the parent appended to it
		r.Metadata().Module = fmt.Sprintf("%s.%s", name, r.Metadata().Module)
		r.Metadata().Module = strings.TrimSuffix(r.Metadata().Module, ".")

		ctx, err := moduleConfig.getContext(r)
		if err != nil {
			panic("no body found for resource")
		}

		bdy, err := moduleConfig.getBody(r)
		if err != nil {
			panic("no body found for resource")
		}

		// depends on is a property of the embedded type we need to set this manually
		err = setDependsOn(ctx, rt, b.Body, dependsOn)
		if err != nil {
			return []error{err}
		}

		err = c.addResource(r, ctx, bdy)
		if err != nil {
			return []error{err}
		}
	}

	return nil
}

func (p *Parser) parseResource(ctx *hcl.EvalContext, c *Config, file string, b *hclsyntax.Block, moduleName string, dependsOn []string, disabled bool) error {
	var rt types.Resource
	var err error

	ignoreErrors := false

	switch b.Type {
	case types.TypeResource:
		// if the type is resource there should be two labels, one for the type and one for the name
		if len(b.Labels) != 2 {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Message = `"invalid formatting for 'resource' stanza, resources should have a name and a type, i.e. 'resource "type" "name" {}'`
			de.Level = errors.ParserErrorLevelError

			return de
		}

		name := b.Labels[1]
		if err := validateResourceName(name); err != nil {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Message = de.Error()
			de.Level = errors.ParserErrorLevelError

			return de
		}

		// PrimativesOnly parse to ResourceBase
		if p.options.PrimativesOnly {
			rt = &types.ResourceBase{
				Meta: types.Meta{
					Name:       name,
					Type:       b.Labels[0],
					Properties: map[string]any{},
				},
			}

			// ignore any errors when parsing
			ignoreErrors = true
		} else {
			rt, err = p.registeredTypes.CreateResource(b.Labels[0], name)
			if err != nil {
				de := &errors.ParserError{}
				de.Line = b.TypeRange.Start.Line
				de.Column = b.TypeRange.Start.Column
				de.Filename = file
				de.Message = fmt.Sprintf("unable to create resource '%s' %s", b.Type, err)
				de.Level = errors.ParserErrorLevelError

				return de
			}
		}

	case resources.TypeLocal:
		// if the type is local check there is one label
		if len(b.Labels) != 1 {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = `invalid formatting for 'local' stanza, resources should have a name and a type, i.e. 'local "name" {}'`

			return de
		}

		name := b.Labels[0]
		if err := validateResourceName(name); err != nil {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = err.Error()

			return de
		}

		rt, err = p.registeredTypes.CreateResource(resources.TypeLocal, name)
		if err != nil {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = fmt.Sprintf(`unable to create local, this error should never happen %s`, err)

			return de
		}

	case resources.TypeOutput:
		// if the type is output check there is one label
		if len(b.Labels) != 1 {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = `invalid formatting for 'output' stanza, resources should have a name and a type, i.e. 'output "name" {}'`

			return de
		}

		name := b.Labels[0]
		if err := validateResourceName(name); err != nil {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = err.Error()

			return de
		}

		rt, err = p.registeredTypes.CreateResource(resources.TypeOutput, name)
		if err != nil {
			de := &errors.ParserError{}
			de.Line = b.TypeRange.Start.Line
			de.Column = b.TypeRange.Start.Column
			de.Filename = file
			de.Level = errors.ParserErrorLevelError
			de.Message = fmt.Sprintf(`unable to create output, this error should never happen %s`, err)

			return de
		}
	}

	rt.Metadata().Module = moduleName
	rt.Metadata().File = file
	rt.Metadata().Line = b.TypeRange.Start.Line
	rt.Metadata().Column = b.TypeRange.Start.Column

	err = decodeBody(ctx, c, file, b, rt, ignoreErrors)
	if err != nil {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = fmt.Sprintf("error creating resource '%s' in file %s: %s", b.Labels[0], file, err)
		return de
	}

	// if we have an output, get the description
	// this is only needed when parsing primatives as
	// this value is normally set during walk
	if rt.Metadata().Type == resources.TypeOutput && b.Body.Attributes["description"] != nil {
		desc, diags := b.Body.Attributes["description"].Expr.Value(ctx)
		if !diags.HasErrors() {
			rt.(*resources.Output).Description = desc.AsString()
		}
	}

	// depends on is a property of the embedded type we need to set this manually
	err = setDependsOn(ctx, rt, b.Body, dependsOn)
	if err != nil {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = fmt.Sprintf(`unable to set depends_on, %s`, err)

		return de
	}

	err = c.addResource(rt, ctx, b.Body)
	if err != nil {
		de := &errors.ParserError{}
		de.Line = b.TypeRange.Start.Line
		de.Column = b.TypeRange.Start.Column
		de.Filename = file
		de.Level = errors.ParserErrorLevelError
		de.Message = fmt.Sprintf(`unable to add resource "%s" to config %s`, resources.FQRNFromResource(rt).String(), err)

		return de
	}

	return nil
}

func setContextVariableIfMissing(ctx *hcl.EvalContext, key string, value cty.Value) {
	if m, ok := ctx.Variables["variable"]; ok {
		if _, ok := m.AsValueMap()[key]; ok {
			return
		}
	}

	setContextVariable(ctx, key, value)
}

func setContextVariable(ctx *hcl.EvalContext, key string, value cty.Value) {
	valMap := map[string]cty.Value{}

	// get the existing map
	if m, ok := ctx.Variables["variable"]; ok {
		valMap = m.AsValueMap()
	}

	valMap[key] = value

	ctx.Variables["variable"] = cty.ObjectVal(valMap)
}

// setContextVariableFromPath sets a context variable using a nested structure based
// on the given path. Will create any child maps needed to satisfy the path.
// i.e "resources.foo.bar" set to "true" would return
// ctx.Variables["resources"].AsValueMap()["foo"].AsValueMap()["bar"].True() = true
func setContextVariableFromPath(ctx *hcl.EvalContext, path string, value cty.Value) error {
	pathParts := strings.Split(path, ".")

	var err error
	ctx.Variables, err = setMapVariableFromPath(ctx.Variables, pathParts, value)

	return err
}

func setMapVariableFromPath(root map[string]cty.Value, path []string, value cty.Value) (map[string]cty.Value, error) {
	// it is possible for root to be nil, ensure this is set to an empty map
	if root == nil {
		root = map[string]cty.Value{}
	}

	// gets the name and the index from the path
	name, index, rPath, err := getNameAndIndex(path)
	if err != nil {
		return nil, err
	}

	// do we have a node at this path if not we need to create if it
	// nodes can either be a map or a list of maps
	val, ok := root[name]
	if !ok {
		if index >= 0 {
			// create a list with the correct length
			vals := make([]cty.Value, index+1)

			val = cty.ListVal(vals)
		} else {
			// create a map nodej
			val = cty.ObjectVal(map[string]cty.Value{".keep": cty.BoolVal(true)})
		}
	}

	if index >= 0 {
		// if we have an index we need to set the list variable for the map at that
		// index and then recursively set the other elements in the map
		updated, err := setListVariableFromPath(val.AsValueSlice(), rPath, index, value)
		if err != nil {
			return nil, err
		}

		root[name] = cty.ListVal(updated)
	} else {
		// check if the value is a list, it is possible that the user is
		// trying to incorrectly access a list type using a string parameter
		// if we do not check this it will panic
		//if val.Type().IsTupleType() || val.Type().IsListType() {
		//	err := fmt.Errorf(`the parameter is a list of items, you can not use the string index "%s" to access items, please use numeric indexes`, name)
		//	return nil, err
		//}

		// if this is the end of the line set the value and return
		if len(rPath) == 0 {
			root[name] = value
			return root, nil
		}

		// we are setting a map, recurse
		updated, err := setMapVariableFromPath(val.AsValueMap(), rPath, value)
		if err != nil {
			return nil, err
		}

		root[name] = cty.ObjectVal(updated)
	}

	return root, nil
}

func setListVariableFromPath(root []cty.Value, path []string, index int, value cty.Value) ([]cty.Value, error) {
	// we have a node but do we need to expand it in size?
	if index >= len(root) {
		root = append(root, make([]cty.Value, index+1-len(root))...)
	}

	var setVal cty.Value
	if len(path) > 0 {

		val := root[index]
		if val.IsNull() {
			val = cty.ObjectVal(map[string]cty.Value{".keep": cty.BoolVal(true)})
		}

		updated, err := setMapVariableFromPath(val.AsValueMap(), path, value)
		if err != nil {
			return nil, err
		}

		setVal = cty.ObjectVal(updated)
	} else {
		setVal = value
	}

	// check the type of the collection, if trying to set a type that is inconsistent
	// from the other types in the collection, return an error
	if len(root) > 0 {
		if root[0].Type() != cty.NilType && root[0].Type().FriendlyName() != setVal.Type().FriendlyName() {
			return nil, fmt.Errorf("lists must contain similar types, you have tried to set a %s, to a list of type %s", value.Type().FriendlyName(), root[0].Type().FriendlyName())
		}
	}

	root[index] = setVal

	// build a unique list of keys and types, if the
	// node contains a list of maps
	ul := map[string]cty.Type{}
	for _, m := range root {
		if m.Type().IsObjectType() || m.Type().IsMapType() {
			for k, v := range m.AsValueMap() {
				ul[k] = v.Type()
			}
		}
	}

	if len(ul) == 0 {
		return root, nil
	}

	// we need to normalize the map collection as cty does not allow inconsistent map keys
	for k, v := range ul {
		for i, m := range root {
			if m.IsNull() {
				m = cty.ObjectVal(map[string]cty.Value{".keep": cty.BoolVal(true)})
			}

			if _, ok := m.AsValueMap()[k]; !ok {
				val := m.AsValueMap()
				val[k] = cty.NullVal(v)
				root[i] = cty.ObjectVal(val)
			}
		}
	}

	return root, nil
}

// gets the name of the path and the index
// if path[0] == foo     and path[1] = bar[0] returns foo, -1, nil
// if path[0] == bar[0]  and path[1] = biz    returns bar, 0, nil
// if path[0] == foo     and path[1] = 0 returns foo, 0, nil
// if path[0] == foo     and path[1] = bar returns foo, -1, nil
// if path[0] == foo     and path[1] = nil returns foo, -1, nil
func getNameAndIndex(path []string) (name string, index int, remainingPath []string, err error) {
	index = -1

	// is the path an array with parenthesis
	rg, _ := regexp.Compile(`(.*)\[(.+)\]`)
	if sm := rg.FindStringSubmatch(path[0]); len(sm) == 3 {
		name = sm[1]

		var convErr error
		index, convErr = strconv.Atoi(sm[2])
		if convErr != nil {
			return "", -1, nil, fmt.Errorf("index %s is not a number", sm[2])
		}

		return name, index, path[1:], nil
	}

	// is the path a number using the . notation for an index
	if len(path) > 1 {
		index, convErr := strconv.Atoi(path[1])
		if convErr == nil {
			return path[0], index, path[2:], nil
		}
	}

	// normal path item
	return path[0], -1, path[1:], nil
}

func buildContext(filePath string, customFunctions map[string]function.Function) *hcl.EvalContext {
	ctx := &hcl.EvalContext{
		Functions: map[string]function.Function{},
		Variables: map[string]cty.Value{},
	}

	valMap := map[string]cty.Value{}
	ctx.Variables["resource"] = cty.ObjectVal(valMap)

	ctx.Functions = getDefaultFunctions(filePath)

	// add the custom functions
	for k, v := range customFunctions {
		ctx.Functions[k] = v
	}

	return ctx
}

func decodeBody(ctx *hcl.EvalContext, config *Config, path string, b *hclsyntax.Block, p any, ignoreErrors bool) error {
	dr, err := getDependentResources(b, ctx, config, p, "")
	if err != nil {
		return err
	}

	// filter the list so that they are unique
	uniqueResources := []string{}
	for _, v := range dr {
		found := false
		for _, r := range uniqueResources {
			if r == v {
				found = true
				break
			}
		}

		if !found {
			uniqueResources = append(uniqueResources, v)
		}
	}

	// if variable process the body, everything else
	// lazy process on dag walk
	if b.Type == string(resources.TypeVariable) {
		diag := gohcl.DecodeBody(b.Body, ctx, p)
		if diag.HasErrors() {
			pe := &errors.ParserError{}
			pe.Column = b.Body.SrcRange.Start.Column
			pe.Line = b.Body.SrcRange.Start.Line
			pe.Filename = b.Body.SrcRange.Filename
			pe.Message = fmt.Sprintf("unable to decode body, %s", err)
			pe.Level = errors.ParserErrorLevelError

			// if ignore errors is false return the parsing error, otherwise
			// swallow it
			if !ignoreErrors {
				return pe
			}
		}
	}

	// set the dependent resources
	res := p.(types.Resource)
	res.Metadata().Links = uniqueResources

	return nil
}

// Recursively checks the fields and blocks on the resource to identify links to other resources
// i.e. resource.container.foo.network[0].name
// when a link is found it is replaced with an empty value of the correct type and the
// dependent resources are returned to be processed later
func getDependentResources(b *hclsyntax.Block, ctx *hcl.EvalContext, c *Config, resource any, path string) ([]string, error) {
	references := []string{}

	for _, a := range b.Body.Attributes {
		refs, err := processExpr(a.Expr)
		if err != nil {
			pe := &errors.ParserError{}
			pe.Column = b.Body.SrcRange.Start.Column
			pe.Line = b.Body.SrcRange.Start.Line
			pe.Filename = b.Body.SrcRange.Filename
			pe.Message = fmt.Sprintf("unable to process attribute %s, %s", a.Name, err)
			pe.Level = errors.ParserErrorLevelError

			return nil, pe
		}

		references = append(references, refs...)
	}

	// we need to keep a count of the current block so that we
	// can get this
	blockIndex := map[string]int{}
	for _, b := range b.Body.Blocks {
		if _, ok := blockIndex[b.Type]; ok {
			blockIndex[b.Type]++
		} else {
			blockIndex[b.Type] = 0
		}

		ref := fmt.Sprintf("%s.%s[%d]", path, b.Type, blockIndex[b.Type])
		ref = strings.TrimPrefix(ref, ".")
		cr, err := getDependentResources(b, ctx, c, resource, ref)
		if err != nil {
			return nil, err
		}

		references = append(references, cr...)
	}

	me := resource.(types.Resource)

	// we have the references, now check that the references
	// are not cyclical
	for _, dep := range references {
		// the references might not exist yet, we are parsing flat
		// but if there is a cyclical reference, one end of the circle will be found
		d, err := c.FindResource(dep)
		//fqdnD := resources.FQDNFromResource(me)
		if err == nil {
			// check the deps on the linked resource
			for _, cdep := range d.Metadata().Links {

				fqrn, err := resources.ParseFQRN(cdep)
				fqrn.Attribute = ""

				// append the parent module to the link as they are relative
				if d.Metadata().Module != "" {
					fqrn.Module = d.Metadata().Module
				}

				if err != nil {
					pe := &errors.ParserError{}
					pe.Column = b.Body.SrcRange.Start.Column
					pe.Line = b.Body.SrcRange.Start.Line
					pe.Filename = b.Body.SrcRange.Filename
					pe.Message = fmt.Sprintf("dependency %s, is not a valid resource", cdep)
					pe.Level = errors.ParserErrorLevelError
					return nil, pe
				}

				if me.Metadata().Name == fqrn.Resource &&
					me.Metadata().Type == fqrn.Type &&
					me.Metadata().Module == fqrn.Module {

					pe := &errors.ParserError{}
					pe.Column = b.Body.SrcRange.Start.Column
					pe.Line = b.Body.SrcRange.Start.Line
					pe.Filename = b.Body.SrcRange.Filename
					pe.Message = fmt.Sprintf("'%s' depends on '%s' which creates a cyclical dependency, remove the dependency from one of the resources", fqrn.String(), d.Metadata().ID)
					pe.Level = errors.ParserErrorLevelError

					return nil, pe
				}
			}
		}
	}

	return references, nil
}

// processAttribute extracts the necessary data out of the HCL
// attribute like a function or resource parameter so we can determine
// which attributes are lazy evaluated due to dependency on another resource.
// Attributes can be nested, therefore this function needs to return an array of
// attributes
// examples:
// something = resource.mine.attr
// something = resource.mine.array.0.attr
// something = env(resource.mine.attr)
// something = "${resource.mine.attr}"
// something = "testing/${resource.mine.attr}"
// something = "testing/${env(resource.mine.attr)}"
// something = resource.mine.attr == "abc" ? resource.mine.attr : "abc"
func processExpr(expr hclsyntax.Expression) ([]string, error) {
	resources := []string{}

	switch ex := expr.(type) {
	// a template is a mix of functions, scope expressions and literals
	// we need to check each part
	case *hclsyntax.TemplateExpr:
		for _, v := range ex.Parts {
			res, err := processExpr(v)
			if err != nil {
				return nil, err
			}

			resources = append(resources, res...)
		}
	case *hclsyntax.TemplateWrapExpr:
		res, err := processExpr(ex.Wrapped)
		if err != nil {
			return nil, err
		}

		resources = append(resources, res...)

	// function call expressions are user defined functions
	// myfunction(resource.container.base.name)
	case *hclsyntax.FunctionCallExpr:
		for _, v := range ex.Args {
			res, err := processExpr(v)
			if err != nil {
				return nil, err
			}

			resources = append(resources, res...)
		}
	// a function can contain args that may also have an expression
	case *hclsyntax.ScopeTraversalExpr:
		ref, err := processScopeTraversal(ex)
		if err != nil {
			return nil, err
		}

		// only add if a resource has been returned
		if ref != "" {
			resources = append(resources, ref)
		}

	case *hclsyntax.ObjectConsExpr:
		for _, v := range ex.Items {
			res, err := processExpr(v.ValueExpr)
			if err != nil {
				return nil, err
			}

			resources = append(resources, res...)
		}
	case *hclsyntax.TupleConsExpr:
		for _, v := range ex.Exprs {
			res, err := processExpr(v)
			if err != nil {
				return nil, err
			}

			resources = append(resources, res...)
		}
	// conditional expressions are like if statements
	// resource.container.base.name == "hello" ? "this" : "that"
	case *hclsyntax.ConditionalExpr:
		conditions, err := processExpr(ex.Condition)
		if err != nil {
			return nil, err
		}
		resources = append(resources, conditions...)

		trueResults, err := processExpr(ex.TrueResult)
		if err != nil {
			return nil, err
		}
		resources = append(resources, trueResults...)

		falseResults, err := processExpr(ex.FalseResult)
		if err != nil {
			return nil, err
		}
		resources = append(resources, falseResults...)
	// binary expressions are two part comparisons
	// resource.container.base.name == "hello"
	// resource.container.base.name != "hello"
	// resource.container.base.name > 3
	case *hclsyntax.BinaryOpExpr:
		lhs, err := processExpr(ex.LHS)
		if err != nil {
			return nil, err
		}
		resources = append(resources, lhs...)

		rhs, err := processExpr(ex.RHS)
		if err != nil {
			return nil, err
		}
		resources = append(resources, rhs...)
	case *hclsyntax.SplatExpr:
		ref, err := processExpr(ex.Source)
		if err != nil {
			return nil, err
		}

		// only add if a resource has been returned
		if len(ref) > 0 {
			resources = append(resources, ref...)
		}

		//default:
		//	pretty.Println(expr)
	}

	return resources, nil
}

func processScopeTraversal(expr *hclsyntax.ScopeTraversalExpr) (string, error) {
	strExpression := ""
	for i, t := range expr.Traversal {
		if i == 0 {
			strExpression += t.(hcl.TraverseRoot).Name

			// if this is not a resource reference quit
			if strExpression != "resource" && strExpression != "module" && strExpression != "local" && strExpression != "output" {
				return "", nil
			}
		} else {
			// does this exist in the context
			switch tt := t.(type) {
			case hcl.TraverseAttr:
				strExpression += "." + tt.Name
			case hcl.TraverseIndex:
				switch tt.Key.Type() {
				case cty.String:
					strExpression += "[\"" + tt.Key.AsString() + "\"]"
				case cty.Number:
					strExpression += "[" + tt.Key.AsBigFloat().String() + "]"
				}
			}
		}
	}

	// add to the references collection and replace with a nil value
	// we will resolve these references before processing
	return strExpression, nil
}

func (p *Parser) process(c *Config) error {
	ce := errors.NewConfigError()

	// process the files and resolve dependency, do this first without any
	// callbacks so we can calculate the checksum
	// we are going to ignore the errors at this stage
	// as there might be interpolation errors
	c.walk(createCallback(
		c,
		func(r types.Resource) error {
			r.Metadata().Checksum.Parsed = generateChecksum(r)
			return nil
		},
	), false)

	// variables are not added to the dag so we need to process these
	// separately
	vars, err := c.FindResourcesByType(resources.TypeVariable)
	if err == nil {
		for _, v := range vars {
			if p.options.Callback != nil {
				if err := p.options.Callback(v); err != nil {
					return err
				}
			}
		}
	}

	// now re-run this time with the callback and the Process function
	// to calculate a final checksum after any computed properties have been
	// set
	errs := c.walk(createCallback(
		c,
		func(r types.Resource) error {
			if p, ok := r.(types.Processable); ok {
				if err := p.Process(); err != nil {
					return err
				}
			}

			if p.options.Callback != nil {
				if err := p.options.Callback(r); err != nil {
					return err
				}
			}

			r.Metadata().Checksum.Processed = generateChecksum(r)
			return nil
		},
	), false)

	for _, e := range errs {
		ce.AppendError(e)
	}

	if len(ce.Errors) > 0 {
		return ce
	}

	return nil
}

// ensureAbsolute ensure that the given path is either absolute or
// if relative is converted to abasolute based on the path of the config
func ensureAbsolute(path, file string) string {
	// if the file starts with a / and we are on windows
	// we should treat this as absolute
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") {
		return filepath.Clean(path)
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	// path is relative so make absolute using the current file path as base
	file, _ = filepath.Abs(file)

	baseDir := file
	// check if the basepath is a file return its directory
	s, _ := os.Stat(file)
	if !s.IsDir() {
		baseDir = filepath.Dir(file)
	}

	fp := filepath.Join(baseDir, path)

	return filepath.Clean(fp)
}

func validateResourceName(name string) error {
	if name == "resource" || name == "module" || name == "output" || name == "variable" {
		return fmt.Errorf("invalid resource name %s, resources can not use the reserved names [resource, module, output, variable]", name)
	}

	invalidChars := `^[0-9]*$`
	r, _ := regexp.Compile(invalidChars)
	if r.MatchString(name) {
		return fmt.Errorf("invalid resource name %s, resources can not be given a numeric identifier", name)
	}

	invalidChars = `[^0-9a-zA-Z_-]`
	r, _ = regexp.Compile(invalidChars)
	if r.MatchString(name) {
		return fmt.Errorf("invalid resource name %s, resources can only contain the characters 0-9 a-z A-Z _ -", name)
	}

	return nil
}
