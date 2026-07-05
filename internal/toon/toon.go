package toon

import (
	"fmt"
	"strings"
	"time"

	toon "github.com/toon-format/toon-go"
)

// FieldDef describes how to extract a field from gh JSON.
type FieldDef struct {
	Type     string
	Key      string
	Subkey   string
	As       string
	Empty    string
	Map      map[string]string
	Fallback string
	Fn       func(item map[string]any) any
}

func Field(key, as string) FieldDef {
	if as == "" {
		as = key
	}
	return FieldDef{Type: "field", Key: key, As: as}
}

func Pluck(key, subkey, as string) FieldDef {
	if as == "" {
		as = key
	}
	return FieldDef{Type: "pluck", Key: key, Subkey: subkey, As: as}
}

func JoinArray(key, subkey, as, empty string) FieldDef {
	if as == "" {
		as = key
	}
	if empty == "" {
		empty = "none"
	}
	return FieldDef{Type: "joinArray", Key: key, Subkey: subkey, As: as, Empty: empty}
}

func RelativeTime(key, as string) FieldDef {
	if as == "" {
		as = key
	}
	return FieldDef{Type: "relativeTime", Key: key, As: as}
}

func BoolYesNo(key, as string) FieldDef {
	if as == "" {
		as = key
	}
	return FieldDef{Type: "boolYesNo", Key: key, As: as}
}

func MapEnum(key string, m map[string]string, fallback, as string) FieldDef {
	if as == "" {
		as = key
	}
	return FieldDef{Type: "mapEnum", Key: key, As: as, Map: m, Fallback: fallback}
}

func Lower(key, as string) FieldDef {
	if as == "" {
		as = key
	}
	return FieldDef{Type: "lower", Key: key, As: as}
}

func ChecksSummary(key, as string) FieldDef {
	if as == "" {
		as = key
	}
	return FieldDef{Type: "checksSummary", Key: key, As: as}
}

func Custom(as string, fn func(item map[string]any) any) FieldDef {
	return FieldDef{Type: "custom", As: as, Fn: fn}
}

// Extract applies schema to a single item.
func Extract(item map[string]any, schema []FieldDef) map[string]any {
	result := make(map[string]any, len(schema))
	for _, def := range schema {
		outputKey := def.As
		if outputKey == "" {
			outputKey = def.Key
		}
		switch def.Type {
		case "field":
			if v, ok := item[def.Key]; ok {
				result[outputKey] = v
			} else {
				result[outputKey] = nil
			}
		case "pluck":
			if obj, ok := item[def.Key].(map[string]any); ok {
				result[outputKey] = obj[def.Subkey]
			} else {
				result[outputKey] = nil
			}
		case "joinArray":
			arr, _ := item[def.Key].([]any)
			if len(arr) > 0 {
				parts := make([]string, 0, len(arr))
				for _, x := range arr {
					switch v := x.(type) {
					case string:
						parts = append(parts, v)
					case map[string]any:
						if s, ok := v[def.Subkey].(string); ok {
							parts = append(parts, s)
						}
					}
				}
				result[outputKey] = strings.Join(parts, ",")
			} else {
				empty := def.Empty
				if empty == "" {
					empty = "none"
				}
				result[outputKey] = empty
			}
		case "relativeTime":
			if s, ok := item[def.Key].(string); ok {
				result[outputKey] = formatRelativeTime(s)
			} else {
				result[outputKey] = "unknown"
			}
		case "boolYesNo":
			result[outputKey] = boolToYesNo(item[def.Key])
		case "mapEnum":
			val, _ := item[def.Key].(string)
			if val != "" {
				if mapped, ok := def.Map[val]; ok {
					result[outputKey] = mapped
				} else if def.Fallback != "" {
					result[outputKey] = def.Fallback
				} else {
					result[outputKey] = val
				}
			} else if def.Fallback != "" {
				result[outputKey] = def.Fallback
			} else {
				result[outputKey] = "none"
			}
		case "lower":
			if s, ok := item[def.Key].(string); ok {
				result[outputKey] = strings.ToLower(s)
			} else {
				result[outputKey] = item[def.Key]
			}
		case "checksSummary":
			checks, _ := item[def.Key].([]any)
			if len(checks) > 0 {
				passed := 0
				for _, c := range checks {
					if m, ok := c.(map[string]any); ok {
						conclusion, _ := m["conclusion"].(string)
						if conclusion == "SUCCESS" || conclusion == "NEUTRAL" {
							passed++
						}
					}
				}
				result[outputKey] = fmt.Sprintf("%d/%d pass", passed, len(checks))
			} else {
				result[outputKey] = "none"
			}
		case "custom":
			if def.Fn != nil {
				result[outputKey] = def.Fn(item)
			}
		}
	}
	return result
}

func boolToYesNo(v any) string {
	if b, ok := v.(bool); ok && b {
		return "yes"
	}
	return "no"
}

// RenderList renders a labeled list as TOON.
func RenderList(label string, items []map[string]any, schema []FieldDef) (string, error) {
	extracted := make([]map[string]any, len(items))
	for i, item := range items {
		extracted[i] = Extract(item, schema)
	}
	return toon.MarshalString(map[string]any{label: extracted})
}

// RenderDetail renders a single labeled detail object as TOON.
func RenderDetail(label string, item map[string]any, schema []FieldDef) (string, error) {
	return toon.MarshalString(map[string]any{label: Extract(item, schema)})
}

// RenderHelp renders help suggestions with manual formatting.
func RenderHelp(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("help[%d]:\n", len(lines)))
	for _, l := range lines {
		b.WriteString("  ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// RenderError renders an error in TOON format.
func RenderError(message, code string, suggestions ...string) (string, error) {
	block, err := toon.MarshalString(map[string]any{"error": message, "code": code})
	if err != nil {
		return "", err
	}
	if len(suggestions) > 0 {
		return block + "\n" + RenderHelp(suggestions), nil
	}
	return block, nil
}

// RenderOutput combines multiple TOON blocks.
func RenderOutput(blocks ...string) string {
	var parts []string
	for _, b := range blocks {
		if b != "" {
			parts = append(parts, b)
		}
	}
	return strings.Join(parts, "\n")
}

// Encode marshals a value as TOON.
func Encode(v any) (string, error) {
	return toon.MarshalString(v)
}

func formatRelativeTime(iso string) string {
	if iso == "" {
		return "unknown"
	}
	then, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "unknown"
	}
	diff := time.Since(then)
	sec := int(diff.Seconds())
	if sec < 60 {
		return "just now"
	}
	min := sec / 60
	if min < 60 {
		return fmt.Sprintf("%dm ago", min)
	}
	hr := min / 60
	if hr < 24 {
		return fmt.Sprintf("%dh ago", hr)
	}
	day := hr / 24
	if day < 30 {
		return fmt.Sprintf("%dd ago", day)
	}
	mon := day / 30
	if mon < 12 {
		return fmt.Sprintf("%dmo ago", mon)
	}
	return fmt.Sprintf("%dy ago", mon/12)
}
