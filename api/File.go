package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"gopress/inceptiondb"
)

type File struct {
	Id string `json:"id"` // todo: this is part of persistence layer/logic

	AuthorId      string `json:"author_id"`
	AuthorNick    string `json:"author_nick"`
	AuthorPicture string `json:"author_picture"`
	BlogId        string `json:"blog_id"`

	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`

	CreatedOn time.Time `json:"createdOn"`
}

func requireFileBlog(ctx context.Context, r *http.Request) (*Blog, error) {
	blogId := strings.TrimSpace(r.URL.Query().Get("blogId"))
	if blogId == "" {
		return nil, invalidBlog("blogId is required")
	}
	blog, _, err := RequireBlogAccess(ctx, blogId)
	return blog, err
}

func validateBlogHeaderFile(ctx context.Context, blogId, fileURL string) error {
	if fileURL == "" {
		return nil
	}
	if !strings.HasPrefix(fileURL, "/files/") {
		return invalidBlog("header_image must reference an uploaded blog image")
	}
	fileId := strings.TrimPrefix(fileURL, "/files/")
	if fileId == "" || strings.ContainsAny(fileId, "/?#") {
		return invalidBlog("header_image is invalid")
	}
	file := &File{}
	if err := GetInceptionClient(ctx).FindOne("files", inceptiondb.FindQuery{Filter: JSON{
		"id": fileId, "blog_id": blogId,
	}}, file); err != nil {
		return invalidBlog("header image does not belong to this blog")
	}
	switch strings.ToLower(file.Mime) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/avif":
		return nil
	default:
		return invalidBlog("header image must be JPEG, PNG, GIF, WebP, or AVIF")
	}
}
