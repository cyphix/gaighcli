package fields

import (
	"sort"
	"strings"

	"github.com/cyphix/gaighcli/internal/errors"
	"github.com/cyphix/gaighcli/internal/toon"
)

// ExtraFieldSpec describes an extra field requestable via --fields.
type ExtraFieldSpec struct {
	JSONKey string
	Def     toon.FieldDef
}

// ParseFieldsResult holds parsed --fields output.
type ParseFieldsResult struct {
	ExtraDefs    []toon.FieldDef
	ExtraJSONKeys []string
}

// ParseFields parses a comma-separated --fields value.
func ParseFields(fieldsArg string, available map[string]ExtraFieldSpec) (ParseFieldsResult, error) {
	if fieldsArg == "" {
		return ParseFieldsResult{}, nil
	}
	seen := make(map[string]bool)
	var requested []string
	for _, f := range strings.Split(fieldsArg, ",") {
		f = strings.TrimSpace(f)
		if f != "" && !seen[f] {
			seen[f] = true
			requested = append(requested, f)
		}
	}
	var unknown []string
	for _, name := range requested {
		if _, ok := available[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		names := make([]string, 0, len(available))
		for k := range available {
			names = append(names, k)
		}
		sort.Strings(names)
		return ParseFieldsResult{}, errors.NewGoAIError(
			"Unknown field(s): "+strings.Join(unknown, ", ")+". Available: "+strings.Join(names, ", "),
			"VALIDATION_ERROR",
		)
	}
	var result ParseFieldsResult
	for _, name := range requested {
		spec := available[name]
		result.ExtraDefs = append(result.ExtraDefs, spec.Def)
		result.ExtraJSONKeys = append(result.ExtraJSONKeys, spec.JSONKey)
	}
	return result, nil
}
