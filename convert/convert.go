package convert

import (
	"fmt"
	"reflect"
	"strconv"
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

		// Transform labeled blocks to support both named and indexed access
		transformLabeledBlocks(reflect.ValueOf(val), ctyMap)

		ctyVal = cty.ObjectVal(ctyMap)
	}

	return ctyVal, nil
}

// transformLabeledBlocks converts slice fields with HCL block+label tags to maps.
// This allows named access like resource.cloud_account.prod.user.admin instead of user[0].
func transformLabeledBlocks(v reflect.Value, ctyMap map[string]cty.Value) {
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
			// No label field, this is not a labeled block - leave it as a list
			continue
		}

		// Build a map with BOTH numeric indices and label keys
		// This allows both user[0] and user.admin access patterns
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

			// Convert element to cty AS ITS ACTUAL TYPE (not ResourceBase)
			elemCty, err := elementToCty(elem)
			if err != nil {
				continue
			}

			// Add by label for named access (user.admin)
			labeledMap[label] = elemCty

			// ALSO add by numeric index as string for indexed access (user.0 or user["0"])
			labeledMap[strconv.Itoa(j)] = elemCty
		}

		// Replace the list with a map in ctyMap for labeled blocks
		// This enables BOTH named access (user.admin.email) AND indexed access (user.0.email)
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

	// Use gocty to convert the element
	typ, err := gocty.ImpliedType(v.Interface())
	if err != nil {
		return cty.NilVal, err
	}

	ctyVal, err := gocty.ToCtyValue(v.Interface(), typ)
	if err != nil {
		return cty.NilVal, err
	}

	// If the value is an object, recursively transform labeled blocks within it
	if ctyVal.Type().IsObjectType() {
		valueMap := ctyVal.AsValueMap()
		transformLabeledBlocks(v, valueMap)
		return cty.ObjectVal(valueMap), nil
	}

	return ctyVal, nil
}

func CtyToGo(val cty.Value, target any) error {
	return gocty.FromCtyValue(val, target)
}
