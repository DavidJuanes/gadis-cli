package report

import (
	"io"
	"os"
)

// ParseFile opens the file name and reads it with parse, one of the parsers
// of this package.
func ParseFile[T any](name string, parse func(io.Reader) (T, error)) (T, error) {
	f, err := os.Open(name) //nolint:gosec // G304: the path comes from the workflow, which owns the runner
	if err != nil {
		var zero T
		return zero, err
	}
	defer func() { _ = f.Close() }() // read-only
	return parse(f)
}
