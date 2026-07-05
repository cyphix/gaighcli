package args

import (
	"regexp"
	"strconv"

	"github.com/cyphix/gaighcli/internal/errors"
)

func flagEqualsPrefix(flag string) string {
	return flag + "="
}

// GetFlag returns a flag value without modifying args.
func GetFlag(args []string, name string) string {
	equalsPrefix := flagEqualsPrefix(name)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == name {
			if i+1 >= len(args) {
				return ""
			}
			return args[i+1]
		}
		if len(arg) > len(equalsPrefix) && arg[:len(equalsPrefix)] == equalsPrefix {
			return arg[len(equalsPrefix):]
		}
	}
	return ""
}

// TakeFlag returns a flag value and removes it from args.
func TakeFlag(args *[]string, flag string) string {
	equalsPrefix := flagEqualsPrefix(flag)
	a := *args
	for i := 0; i < len(a); i++ {
		arg := a[i]
		if arg == flag {
			var val string
			if i+1 < len(a) {
				val = a[i+1]
				a = append(a[:i], a[i+2:]...)
			} else {
				a = append(a[:i], a[i+1:]...)
			}
			*args = a
			return val
		}
		if len(arg) > len(equalsPrefix) && arg[:len(equalsPrefix)] == equalsPrefix {
			val := arg[len(equalsPrefix):]
			a = append(a[:i], a[i+1:]...)
			*args = a
			return val
		}
	}
	return ""
}

// HasFlag reports whether a boolean flag is present.
func HasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// TakeBoolFlag reports whether a boolean flag is present and removes it.
func TakeBoolFlag(args *[]string, flag string) bool {
	a := *args
	for i, arg := range a {
		if arg == flag {
			a = append(a[:i], a[i+1:]...)
			*args = a
			return true
		}
	}
	return false
}

// GetAllFlags collects all values for a repeatable flag.
func GetAllFlags(args []string, flag string) []string {
	var result []string
	equalsPrefix := flagEqualsPrefix(flag)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == flag && i+1 < len(args) {
			result = append(result, args[i+1])
			i++
		} else if len(arg) > len(equalsPrefix) && arg[:len(equalsPrefix)] == equalsPrefix {
			result = append(result, arg[len(equalsPrefix):])
		}
	}
	return result
}

// GetPositional returns the first non-flag arg from startIndex.
func GetPositional(args []string, startIndex int) string {
	for i := startIndex; i < len(args); i++ {
		if len(args[i]) == 0 || args[i][0] != '-' {
			return args[i]
		}
	}
	return ""
}

// RequireNumber parses a required numeric argument.
func RequireNumber(raw, label string) (int, error) {
	if raw == "" {
		return 0, errors.NewGoAIError("Missing "+label+" number", "VALIDATION_ERROR")
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.NewGoAIError("Invalid "+label+" number: "+raw, "VALIDATION_ERROR")
	}
	return n, nil
}

var digitsRe = regexp.MustCompile(`^\d+$`)

// TakeNumber finds the first numeric positional, removes it, and returns it.
func TakeNumber(args *[]string, label string) (int, error) {
	a := *args
	var raw string
	var idx = -1
	for i, arg := range a {
		if digitsRe.MatchString(arg) {
			raw = arg
			idx = i
			break
		}
	}
	if idx == -1 {
		return 0, errors.NewGoAIError("Missing "+label+" number", "VALIDATION_ERROR")
	}
	a = append(a[:idx], a[idx+1:]...)
	*args = a
	return strconv.Atoi(raw)
}
