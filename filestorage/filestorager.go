package filestorage

import (
	"io"
)

type Filestorager interface {
	OpenReader(filename string) (io.ReadCloser, error)
	OpenWriter(filename string) (io.WriteCloser, error)
	// TODO: PublicURL(filename string) string ??
}
