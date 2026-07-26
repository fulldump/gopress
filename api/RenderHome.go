package api

import (
	"context"
	"fmt"
	"net/http"

	"gopress/templates"
)

func RenderHome(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	offset, err := listOffset(r)
	if err != nil {
		return err
	}
	page, err := publicArticlePage(ctx, JSON{"published": true, "$ne": JSON{"banned": true}}, offset)
	if err != nil {
		return blogStorageError("render home articles", err)
	}
	if err := templates.Execute(ctx, "home", w, map[string]any{
		"articles": page.Items,
		"nextURL":  nextPageURL(r, page.NextOffset, page.HasMore),
	}); err != nil {
		return fmt.Errorf("error rendering home: %w", err)
	}
	return nil
}
