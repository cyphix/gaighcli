package stdin

import (
	"io"
	"os"
)

// ReadAll reads all of stdin as a UTF-8 string.
func ReadAll() (string, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsTTY reports whether stdin is an interactive terminal.
func IsTTY() bool {
 fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
