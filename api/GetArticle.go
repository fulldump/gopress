package api

import (
	"context"
)

func GetArticle(ctx context.Context) any {
	article, _, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return err
	}
	return article
}
