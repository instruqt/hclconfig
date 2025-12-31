package convert

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"go.instruqt.com/hclconfig/types"
)

func GoToCtyValue(val any) (cty.Value, error) {
	typ, err := gocty.ImpliedType(val)
	if err != nil {
		return cty.False, err
	}

	ctyVal, err := gocty.ToCtyValue(val, typ)
	if err != nil {
		return cty.False, err
	}

	if r, ok := val.(types.Resource); ok {
		ctyMap := ctyVal.AsValueMap()

		// add disabled to the parent
		ctyMap["disabled"] = cty.BoolVal(r.GetDisabled())

		// add depends_on to the parent
		depTyp, err := gocty.ImpliedType(r.GetDependencies())
		if err != nil {
			return cty.False, err
		}

		dep, err := gocty.ToCtyValue(r.GetDependencies(), depTyp)
		if err != nil {
			return cty.False, fmt.Errorf("unable to convert depends_on to cty: %s", err)
		}
		ctyMap["depends_on"] = dep

		// add the meta properties to the parent
		typ, err := gocty.ImpliedType(r.Metadata())
		if err != nil {
			return cty.False, err
		}

		metaVal, err := gocty.ToCtyValue(r.Metadata(), typ)
		if err != nil {
			return cty.False, err
		}

		ctyMap["meta"] = metaVal

		// Transform labeled block slices to maps for named access
		// Pass the parent resource ID to construct full reference paths for nested elements
		transformLabeledBlocks(reflect.ValueOf(val), ctyMap, r.Metadata().ID)

		ctyVal = cty.ObjectVal(ctyMap)
	}

	return ctyVal, nil
}

// transformLabeledBlocks converts slice fields with HCL block+label tags to maps.
// This allows named access like resource.aws_account.test.user.admin instead of user[0].
// parentID is the resource ID (e.g., "resource.azure_subscription.demo") used to construct
// full reference paths for nested elements.
func transformLabeledBlocks(v reflect.Value, ctyMap map[string]cty.Value, parentID string) {
	// Dereference pointer
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Check if this is a slice with hcl block tag
		hclTag := field.Tag.Get("hcl")
		if hclTag == "" || !strings.Contains(hclTag, ",block") {
			continue
		}

		// Get the HCL field name
		hclName := strings.Split(hclTag, ",")[0]
		if hclName == "" {
			continue
		}

		// Check if it's a slice
		if fieldVal.Kind() != reflect.Slice {
			continue
		}

		// Get element type and check for label field
		elemType := fieldVal.Type().Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		if elemType.Kind() != reflect.Struct {
			continue
		}

		labelFieldIdx := findLabelFieldIndex(elemType)
		if labelFieldIdx == -1 {
			continue
		}

		// Build a map keyed by label value
		labeledMap := make(map[string]cty.Value)
		for j := 0; j < fieldVal.Len(); j++ {
			elem := fieldVal.Index(j)
			if elem.Kind() == reflect.Ptr {
				if elem.IsNil() {
					continue
				}
				elem = elem.Elem()
			}

			labelVal := elem.Field(labelFieldIdx)
			if labelVal.Kind() != reflect.String {
				continue
			}

			label := labelVal.String()
			if label == "" {
				continue
			}

			// Convert element to cty
			elemCty, err := elementToCty(elem)
			if err != nil {
				continue
			}

			// Add meta, disabled, and depends_on fields for this nested element
			// This allows elements to be decoded as types.ResourceBase when referenced
			elemMap := elemCty.AsValueMap()
			fullID := fmt.Sprintf("%s.%s.%s", parentID, hclName, label)
			// Meta struct has many fields, all need to be present for gocty decoding
			elemMap["meta"] = cty.ObjectVal(map[string]cty.Value{
				"id":     cty.StringVal(fullID),
				"type":   cty.StringVal(hclName),
				"name":   cty.StringVal(label),
				"module": cty.StringVal(""),
				"file":   cty.StringVal(""),
				"line":   cty.NumberIntVal(0),
				"column": cty.NumberIntVal(0),
				"checksum": cty.ObjectVal(map[string]cty.Value{
					"parsed":    cty.StringVal(""),
					"processed": cty.StringVal(""),
				}),
			})
			// ResourceBase also requires disabled and depends_on fields
			elemMap["disabled"] = cty.BoolVal(false)
			elemMap["depends_on"] = cty.ListValEmpty(cty.String)
			elemCty = cty.ObjectVal(elemMap)

			labeledMap[label] = elemCty
		}

		// Replace the list with a map in ctyMap
		if len(labeledMap) > 0 {
			ctyMap[hclName] = cty.ObjectVal(labeledMap)
		}
	}
}

// findLabelFieldIndex finds the index of the field with hcl:",label" tag
func findLabelFieldIndex(t reflect.Type) int {
	for i := 0; i < t.NumField(); i++ {
		hclTag := t.Field(i).Tag.Get("hcl")
		if strings.Contains(hclTag, ",label") {
			return i
		}
	}
	return -1
}

// elementToCty converts a struct element to a cty.Value
func elementToCty(v reflect.Value) (cty.Value, error) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return cty.NilVal, fmt.Errorf("nil pointer")
		}
		v = v.Elem()
	}

	if !v.CanInterface() {
		return cty.NilVal, fmt.Errorf("cannot interface")
	}

	typ, err := gocty.ImpliedType(v.Interface())
	if err != nil {
		return cty.NilVal, err
	}

	return gocty.ToCtyValue(v.Interface(), typ)
}

func CtyToGo(val cty.Value, target any) error {
	return gocty.FromCtyValue(val, target)
}
