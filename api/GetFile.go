package api

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/fulldump/box"

	"gopress/filestorage"
	"gopress/glueauth"
	"gopress/inceptiondb"
)

func GetFile(w http.ResponseWriter, ctx context.Context) error {

	fileId := box.GetUrlParameter(ctx, "fileId")

	file := &File{}

	err := GetInceptionClient(ctx).FindOne("files", inceptiondb.FindQuery{Filter: JSON{"id": fileId}}, file)
	if err != nil {
		log.Println(err.Error())
		return fileNotFound()
	}
	if file.BlogId == "" {
		return fileNotFound()
	}
	blog := &Blog{}
	if err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": file.BlogId}}, blog); err != nil {
		return fileNotFound()
	}
	if blog.Banned {
		auth := glueauth.GetAuth(ctx)
		if auth == nil || auth.User.ID != blog.OwnerId {
			return fileNotFound()
		}
	}

	fs := box.GetBoxContext(ctx).Action.GetAttribute("filestorager").(filestorage.Filestorager)
	r, err := fs.OpenReader(fileId)
	if err != nil {
		log.Println(err.Error())
		return fileNotFound()
	}

	w.Header().Set("Content-Type", file.Mime) // todo: only if not empty?

	io.Copy(w, r) // todo: handle error properly

	return nil
}

func fileNotFound() HttpError {
	return HttpError{Status: http.StatusNotFound, Title: "File Not Found", Description: "El archivo no existe"}
}
