package api

import (
	"context"
	"log"
	"net/http"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
)

func RetrieveFile(ctx context.Context, r *http.Request) (*File, error) {

	fileId := box.GetUrlParameter(ctx, "fileId")
	blog, err := requireFileBlog(ctx, r)
	if err != nil {
		return nil, err
	}

	query := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"id":      fileId,
			"blog_id": blog.Id,
		},
	}

	response := &File{}

	err = GetInceptionClient(ctx).FindOne("files", query, response)
	if err != nil {
		log.Println(err.Error())
		return nil, ErrorPersistenceRead
	}

	return response, nil
}
