package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"gopress/inceptiondb"
)

const listPageSize = 20

func EnsureArticlePaginationIndexes(db *inceptiondb.Client) error {
	for _, options := range []inceptiondb.IndexOptions{
		{Name: "articles-public-publish-v2", Type: "btree", Fields: []string{"publishOn", "id"}, Unique: true},
		{Name: "articles-admin-updated-v2", Type: "btree", Fields: []string{"updatedOn", "id"}, Unique: true},
	} {
		if err := db.EnsureIndex("articles", &options); err != nil {
			return fmt.Errorf("ensure index %s on articles: %w", options.Name, err)
		}
	}
	return nil
}

type ListPage[T any] struct {
	Items      []T  `json:"items"`
	HasMore    bool `json:"hasMore"`
	NextOffset int  `json:"nextOffset"`
}

func listOffset(r *http.Request) (int, error) {
	value := r.URL.Query().Get("offset")
	if value == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(value)
	if err != nil || offset < 0 {
		return 0, HttpError{Status: http.StatusBadRequest, Title: "Invalid Offset", Description: "offset must be a non-negative integer"}
	}
	return offset, nil
}

func pageSlice[T any](items []T, offset int) ListPage[T] {
	if offset > len(items) {
		offset = len(items)
	}
	end := offset + listPageSize
	if end > len(items) {
		end = len(items)
	}
	pageItems := make([]T, end-offset)
	copy(pageItems, items[offset:end])
	return ListPage[T]{Items: pageItems, HasMore: end < len(items), NextOffset: end}
}

func pageFromLookahead[T any](items []T, offset int) ListPage[T] {
	if items == nil {
		items = []T{}
	}
	hasMore := len(items) > listPageSize
	if hasMore {
		items = items[:listPageSize]
	}
	return ListPage[T]{Items: items, HasMore: hasMore, NextOffset: offset + len(items)}
}

func nextPageURL(r *http.Request, pageOffset int, hasMore bool) string {
	if !hasMore {
		return ""
	}
	query := r.URL.Query()
	query.Set("offset", strconv.Itoa(pageOffset))
	return (&url.URL{Path: r.URL.Path, RawQuery: query.Encode()}).String()
}

func publicArticlePage(ctx context.Context, filter JSON, offset int) (ListPage[*Article], error) {
	db := GetInceptionClient(ctx)
	preferred := preferredContentLanguage(ctx)
	blogs := map[string]*Blog{}
	missingBlogs := map[string]bool{}
	items := make([]*Article, 0, listPageSize+1)
	logicalIndex := 0
	rawOffset := 0

	for len(items) <= listPageSize {
		batch := make([]*Article, 0, listPageSize+1)
		err := db.FindAll("articles", inceptiondb.FindQuery{
			Index: "articles-public-publish-v2", Skip: rawOffset, Limit: listPageSize + 1, Reverse: true, Filter: filter,
		}, func(article *Article) {
			batch = append(batch, article)
		})
		if err != nil {
			return ListPage[*Article]{}, err
		}
		if len(batch) == 0 {
			break
		}
		rawOffset += len(batch)
		for _, article := range batch {
			blog := blogs[article.BlogId]
			if blog == nil && !missingBlogs[article.BlogId] {
				candidate := &Blog{}
				if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{
					"id": article.BlogId, "$ne": JSON{"banned": true},
				}}, candidate); err != nil {
					missingBlogs[article.BlogId] = true
				} else {
					blog = candidate
					blogs[article.BlogId] = candidate
				}
			}
			if blog == nil {
				continue
			}
			if logicalIndex < offset {
				logicalIndex++
				continue
			}
			items = append(items, preparePublicArticle(article, blog, preferred))
			logicalIndex++
			if len(items) > listPageSize {
				break
			}
		}
		if len(batch) < listPageSize+1 {
			break
		}
	}
	return pageFromLookahead(items, offset), nil
}
