package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fulldump/box"

	"gopress/templates"
)

func RenderTag(w http.ResponseWriter, r *http.Request, ctx context.Context) error {
	tag := box.GetUrlParameter(ctx, "tag")
	offset, err := listOffset(r)
	if err != nil {
		return err
	}
	page, err := publicArticlePage(ctx, JSON{"tags": tag, "published": true, "$ne": JSON{"banned": true}}, offset)
	if err != nil {
		return blogStorageError("render tag articles", err)
	}
	if offset == 0 && len(page.Items) == 0 {
		return HttpError{
			Status:      http.StatusNotFound,
			Title:       "Tag Not Found",
			Description: fmt.Sprintf("El tag '%s' todavía no existe", tag),
		}
	}

	w.Header().Set("X-Robots-Tag", "noindex")
	err = templates.Execute(ctx, "tag", w, map[string]any{
		"tag":      tag,
		"articles": page.Items,
		"nextURL":  nextPageURL(r, page.NextOffset, page.HasMore),
	})
	if err != nil {
		return fmt.Errorf("error rendering tag: %w", err)
	}
	return nil
}
