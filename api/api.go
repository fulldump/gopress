package api

import (
	"net/http"
	"time"

	"github.com/fulldump/box"
)

type Article struct {
	Title   string `json:"title"`
	Content string `json:"content"`

	CreatedOn time.Time `json:"createdOn"`
	Published bool      `json:"published"` // todo: use date to program publishment in the future
}

type ArticleShort struct {
	Id    string `json:"id"`
	Title string `json:"title"`
}

func NewApi() *box.B {

	articles := map[string]*Article{}

	b := box.NewBox()

	b.Handle("GET", "/v1/articles", func() any {

		result := []*ArticleShort{}

		for id, article := range articles {
			result = append(result, &ArticleShort{
				Id:    id,
				Title: article.Title,
			})
		}

		return result
	})

	type CreateArticleRequest struct {
		Id    string `json:"id"`
		Title string `json:"title"`
	}

	b.Handle("POST", "/v1/articles", func(input *CreateArticleRequest) any {

		if _, exist := articles[input.Id]; exist {
			return map[string]interface{}{
				"error": "article id '" + input.Id + "' already exists",
			}
		}

		newArticle := &Article{
			Title:     input.Title,
			Content:   "Start here",
			CreatedOn: time.Now(),
			Published: false,
		}

		articles[input.Id] = newArticle

		return newArticle
	})

	b.Handle("GET", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: get article"
	})

	b.Handle("PATCH", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: modify article"
	})

	b.Handle("DELETE", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: delete article"
	})

	b.Handle("POST", "/v1/articles/{articleId}/publish", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: publish article"
	})

	return b
}
