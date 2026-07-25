package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/fulldump/box"
)

func RenderUserTag(w http.ResponseWriter, r *http.Request, ctx context.Context) error {
	blog, err := findPersonalBlogByNick(ctx, box.GetUrlParameter(ctx, "userNick"))
	if err != nil {
		return err
	}
	target := "/blog/" + url.PathEscape(blog.Slug) + "/tag/" + url.PathEscape(box.GetUrlParameter(ctx, "tag"))
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
	return nil
}
