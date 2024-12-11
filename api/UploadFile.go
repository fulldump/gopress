package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/filestorage"
	"gopress/glueauth"
)

type UploadFileOutput struct {
	Files []*File `json:"files"`
}

var ErrorUploadFilesMultipart = errors.New("multipart method is required")
var maxUploadBytes = int64(25 * 1024 * 1024)
var ErrorMaxUploadSize = errors.New(fmt.Sprintf("file size should be less than %d bytes", maxUploadBytes))

var ErrorPersistenceWrite = errors.New("unexpected internal error writing to persistence layer")
var ErrorPersistenceRead = errors.New("unexpected internal error reading from persistence layer")

func UploadFile(w http.ResponseWriter, r *http.Request, ctx context.Context) (any, error) {

	auth := glueauth.GetAuth(ctx)

	response := &UploadFileOutput{
		Files: []*File{},
	}

	m, err := r.MultipartReader()
	if err != nil {
		log.Println(err.Error())
		return nil, ErrorUploadFilesMultipart
	}

	fs := box.GetBoxContext(ctx).Action.GetAttribute("filestorager").(filestorage.Filestorager)

	for {
		part, err := m.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Println(err.Error())
			break // todo: previously was continue (too risky?)
		}

		name := part.FormName()
		mime := part.Header.Get("Content-Type")
		log.Printf("Name: %s; Mime: %s", name, mime)

		fileId := "file_" + uuid.New().String()

		w, err := fs.OpenWriter(fileId)
		if err != nil {
			log.Println(err.Error())
			return nil, ErrorPersistenceWrite
		}

		n, err := copyMaxBytes(w, part, maxUploadBytes)
		if err != nil {
			log.Println(err.Error())
			return nil, ErrorPersistenceWrite
		}
		if n == maxUploadBytes {
			log.Println(ErrorMaxUploadSize)
			return nil, ErrorMaxUploadSize
		}

		now := time.Now().UTC()

		file := &File{
			Id:            fileId,
			AuthorId:      auth.User.ID,
			AuthorNick:    auth.User.Nick,
			AuthorPicture: auth.User.Picture,
			Name:          name,
			Size:          n,
			Mime:          mime,
			CreatedOn:     now,
		}
		response.Files = append(response.Files, file)

		err = GetInceptionClient(ctx).Insert("files", file)
		if err != nil {
			log.Println(err.Error())
			return nil, ErrorPersistenceWrite
		}

	}

	w.WriteHeader(http.StatusCreated)

	return JSON{
		"success": 1,
		"file": JSON{
			"url": "/files/" + response.Files[0].Id,
		},
	}, nil
}

func copyMaxBytes(w io.WriteCloser, r io.ReadCloser, max int64) (int64, error) {

	n, err := io.CopyN(w, r, max)
	if err == io.EOF {
		// All is OK
	} else if err != nil {
		return n, err
	}

	err = w.Close()
	if err != nil {
		return 0, err
	}

	return n, nil
}
