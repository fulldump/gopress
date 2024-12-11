package api

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/fulldump/box"

	"gopress/filestorage"
	"gopress/inceptiondb"
)

func GetFile(w http.ResponseWriter, ctx context.Context) error {

	fileId := box.GetUrlParameter(ctx, "fileId")

	file := &File{}

	err := GetInceptionClient(ctx).FindOne("files", inceptiondb.FindQuery{Filter: JSON{"id": fileId}}, file)
	if err != nil {
		log.Println(err.Error())
		return errors.New("file not found")
	}

	fs := box.GetBoxContext(ctx).Action.GetAttribute("filestorager").(filestorage.Filestorager)
	r, err := fs.OpenReader(fileId)
	if err != nil {
		log.Println(err.Error())
		return errors.New("file not found")
	}

	w.Header().Set("Content-Type", file.Mime) // todo: only if not empty?

	io.Copy(w, r) // todo: handle error properly

	return nil
}
