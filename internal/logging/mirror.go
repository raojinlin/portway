package logging

import (
	"errors"
	"io"
)

// Mirror writes to every destination even if one fails. Desktop apps may have
// no console (notably Windows GUI executables), which must not disable file logs.
func Mirror(writers ...io.Writer) io.Writer { return mirrorWriter(writers) }

type mirrorWriter []io.Writer

func (writers mirrorWriter) Write(data []byte) (int, error) {
	var written int
	var result error
	for _, writer := range writers {
		n, err := writer.Write(data)
		if n > written {
			written = n
		}
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
		result = errors.Join(result, err)
	}
	return written, result
}
