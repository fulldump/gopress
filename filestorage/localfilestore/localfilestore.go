package localfilestore

import (
	"io"
	"os"
	"path"
)

type LocalFilestore struct {
	basedir string
}

func New(basedir string) (*LocalFilestore, error) {

	// Return filestorager!!
	return &LocalFilestore{
		basedir: basedir,
	}, nil
}

func (f *LocalFilestore) OpenWriter(filename string) (io.WriteCloser, error) {

	filename = path.Join(f.basedir, filename)

	// Ensure directories
	basename := path.Dir(filename)
	if err := os.MkdirAll(basename, 0700); err != nil {
		return nil, err
	}

	w, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return w, nil
}

func (f *LocalFilestore) OpenReader(filename string) (io.ReadCloser, error) {

	filename = path.Join(f.basedir, filename)

	r, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	return r, nil
}
