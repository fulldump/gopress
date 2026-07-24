package api

import (
	"context"
	"fmt"
	"net/http"

	"gopress/templates"
)

func RenderBookmarks(w http.ResponseWriter, ctx context.Context) error {
	if err := templates.Execute(ctx, "bookmarks", w, JSON{}); err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}
	return nil
}
