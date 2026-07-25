package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
)

func RenderUser(w http.ResponseWriter, r *http.Request, ctx context.Context) error {
	blog, err := findPersonalBlogByNick(ctx, box.GetUrlParameter(ctx, "userNick"))
	if err != nil {
		return err
	}
	http.Redirect(w, r, "/blog/"+url.PathEscape(blog.Slug), http.StatusPermanentRedirect)
	return nil
}

func findPersonalBlogByNick(ctx context.Context, nick string) (*Blog, error) {
	blog := &Blog{}
	err := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
		"personal": true,
		"$or":      []JSON{{"owner_nick": nick}, {"legacy_nicks": nick}},
		"$ne":      JSON{"banned": true},
	}}, blog)
	if err != nil {
		return nil, blogNotFound()
	}
	return blog, nil
}
