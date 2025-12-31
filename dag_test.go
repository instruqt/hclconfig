package hclconfig

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.instruqt.com/hclconfig/errors"
	"go.instruqt.com/hclconfig/test_fixtures/structs"
)

// testLabeledItem is a test struct with an HCL label field
type testLabeledItem struct {
	Name  string `hcl:",label"`
	Value int    `hcl:"value"`
}

// testNoLabelItem is a test struct without a label field
type testNoLabelItem struct {
	ID    int    `hcl:"id"`
	Value string `hcl:"value"`
}

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

func TestFindSliceElementByLabel_FindsMatchingElement(t *testing.T) {
	items := []testLabeledItem{
		{Name: "first", Value: 1},
		{Name: "second", Value: 2},
		{Name: "third", Value: 3},
	}

	v := reflect.ValueOf(items)
	result, found := findSliceElementByLabel(v, "second")

	require.True(t, found)
	require.Equal(t, "second", result.FieldByName("Name").String())
	require.Equal(t, int64(2), result.FieldByName("Value").Int())
}

func TestFindSliceElementByLabel_NotFoundReturnsEmpty(t *testing.T) {
	items := []testLabeledItem{
		{Name: "first", Value: 1},
		{Name: "second", Value: 2},
	}

	v := reflect.ValueOf(items)
	_, found := findSliceElementByLabel(v, "nonexistent")

	require.False(t, found)
}

func TestFindSliceElementByLabel_EmptySliceReturnsFalse(t *testing.T) {
	items := []testLabeledItem{}

	v := reflect.ValueOf(items)
	_, found := findSliceElementByLabel(v, "anything")

	require.False(t, found)
}

func TestFindSliceElementByLabel_NoLabelFieldReturnsFalse(t *testing.T) {
	items := []testNoLabelItem{
		{ID: 1, Value: "one"},
		{ID: 2, Value: "two"},
	}

	v := reflect.ValueOf(items)
	_, found := findSliceElementByLabel(v, "one")

	require.False(t, found)
}

func TestFindSliceElementByLabel_PointerSliceWorks(t *testing.T) {
	items := []*testLabeledItem{
		{Name: "first", Value: 1},
		{Name: "second", Value: 2},
	}

	v := reflect.ValueOf(items)
	result, found := findSliceElementByLabel(v, "second")

	require.True(t, found)
	// Result is a pointer, dereference to check
	require.Equal(t, "second", result.Elem().FieldByName("Name").String())
}

func TestFindSliceElementByLabel_NilPointerInSliceSkipped(t *testing.T) {
	items := []*testLabeledItem{
		nil,
		{Name: "second", Value: 2},
	}

	v := reflect.ValueOf(items)
	result, found := findSliceElementByLabel(v, "second")

	require.True(t, found)
	require.Equal(t, "second", result.Elem().FieldByName("Name").String())
}

func TestFindSliceElementByLabel_NonStructSliceReturnsFalse(t *testing.T) {
	items := []string{"first", "second", "third"}

	v := reflect.ValueOf(items)
	_, found := findSliceElementByLabel(v, "second")

	require.False(t, found)
}

func TestValidateAttribute_SliceWithNamedLookup(t *testing.T) {
	// Create a struct that contains a slice of labeled items
	type container struct {
		Items []testLabeledItem `hcl:"item,block"`
	}

	c := container{
		Items: []testLabeledItem{
			{Name: "alpha", Value: 10},
			{Name: "beta", Value: 20},
		},
	}

	v := reflect.ValueOf(&c)
	typ := reflect.TypeOf(&c)

	// Should find "item.alpha" through named lookup
	err := validateAttribute(v, typ, []string{"item", "alpha"})
	require.NoError(t, err)

	// Should find "item.beta.value"
	err = validateAttribute(v, typ, []string{"item", "beta", "value"})
	require.NoError(t, err)
}

func TestValidateAttribute_SliceNamedLookupNotFound(t *testing.T) {
	type container struct {
		Items []testLabeledItem `hcl:"item,block"`
	}

	c := container{
		Items: []testLabeledItem{
			{Name: "alpha", Value: 10},
		},
	}

	v := reflect.ValueOf(&c)
	typ := reflect.TypeOf(&c)

	// Should fail to find "item.nonexistent"
	err := validateAttribute(v, typ, []string{"item", "nonexistent"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid list index")
}

func TestValidateAttribute_SliceNumericIndex(t *testing.T) {
	type container struct {
		Items []testLabeledItem `hcl:"item,block"`
	}

	c := container{
		Items: []testLabeledItem{
			{Name: "alpha", Value: 10},
			{Name: "beta", Value: 20},
		},
	}

	v := reflect.ValueOf(&c)
	typ := reflect.TypeOf(&c)

	err := validateAttribute(v, typ, []string{"item", "0"})
	require.NoError(t, err)

	err = validateAttribute(v, typ, []string{"item", "1", "value"})
	require.NoError(t, err)
}
