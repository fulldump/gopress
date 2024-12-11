package api

import (
	"context"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func ListFiles(ctx context.Context) ([]*File, error) {

	auth := glueauth.GetAuth(ctx)

	response := []*File{}

	query := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"author_id": auth.User.ID,
		},
	}

	GetInceptionClient(ctx).FindAll("files", query, func(file *File) {
		response = append(response, file)
	})

	return response, nil
}
