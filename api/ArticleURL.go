package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"gopress/inceptiondb"
)

func availableArticleURL(db *inceptiondb.Client, blogId, articleId, base string) (string, error) {
	for number := 1; number <= 1000; number++ {
		candidate := base
		if number > 1 {
			suffix := fmt.Sprintf("-%d", number)
			candidate = truncateSlug(base, 80-len(suffix)) + suffix
		}
		available, err := articleURLAvailable(db, blogId, articleId, candidate)
		if err != nil {
			return "", err
		}
		if available {
			return candidate, nil
		}
	}
	return "", errors.New("could not derive an available article URL")
}

func articleURLAvailable(db *inceptiondb.Client, blogId, articleId, url string) (bool, error) {
	existing := &Article{}
	err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{"blog_id": blogId, "url": url}}, existing)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return existing.Id == articleId, nil
}

func articleURLConflict() HttpError {
	return HttpError{Status: http.StatusConflict, Title: "Article URL Already Exists", Code: "article_url_conflict",
		Description: "Ya existe un artículo con esa URL en este blog"}
}
