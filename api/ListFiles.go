package api

import (
	"context"
	"net/http"

	"gopress/inceptiondb"
)

func ListFiles(ctx context.Context, r *http.Request) ([]*File, error) {
	blog, err := requireFileBlog(ctx, r)
	if err != nil {
		return nil, err
	}

	response := []*File{}

	query := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"blog_id": blog.Id,
		},
	}

	GetInceptionClient(ctx).FindAll("files", query, func(file *File) {
		response = append(response, file)
	})

	return response, nil
}
