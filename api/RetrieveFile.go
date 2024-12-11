package api

import (
	"context"
	"log"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func RetrieveFile(ctx context.Context) (*File, error) {

	fileId := box.GetUrlParameter(ctx, "fileId")
	auth := glueauth.GetAuth(ctx)

	query := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"id":        fileId,
			"author_id": auth.User.ID,
		},
	}

	response := &File{}

	err := GetInceptionClient(ctx).FindOne("files", query, response)
	if err != nil {
		log.Println(err.Error())
		return nil, ErrorPersistenceRead
	}

	return response, nil
}
