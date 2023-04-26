package api

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/fulldump/box"

	"gopress/templates"
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

type JSON map[string]any

func NewApi(articles map[string]*Article) *box.B {

	b := box.NewBox()

	b.WithInterceptors(func(next box.H) box.H {
		return func(ctx context.Context) {
			r := box.GetRequest(ctx)

			action := box.GetBoxContext(ctx).Action
			actionName := ""
			if action != nil {
				actionName = action.Name
			}

			log.Println(r.Method, r.URL.String(), actionName)
			next(ctx)
		}
	})

	templateHome, err := template.New("").Parse(templates.Home)
	if err != nil {
		panic(err) // todo: handle this properly
	}

	b.Handle("GET", "/", func(w http.ResponseWriter) {
		// todo: limit page size to 10
		// todo: sort by date DESC

		err := templateHome.ExecuteTemplate(w, "", map[string]any{
			"articles": articles,
		})

		if err != nil {
			log.Println("Error rendering home:", err.Error())
		}
	}).WithName("RenderHome")

	templateArticle, err := template.New("").Parse(templates.Article)
	if err != nil {
		panic(err) // todo: handle this properly
	}

	b.Handle("GET", "/articles/{articleId}", func(w http.ResponseWriter, ctx context.Context) {

		articleId := box.GetUrlParameter(ctx, "articleId")

		article, exist := articles[articleId]
		if !exist {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Article not found"))
			return
		}

		err := templateArticle.ExecuteTemplate(w, "", map[string]any{
			"article": article,
		})

		if err != nil {
			log.Println("Error rendering home:", err.Error())
		}
	})

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

	b.Handle("GET", "/v1/articles/{articleId}", func(ctx context.Context, w http.ResponseWriter) any {

		articleId := box.GetUrlParameter(ctx, "articleId")

		article, exist := articles[articleId]
		if !exist {
			w.WriteHeader(http.StatusNotFound)
			return JSON{
				"error": "article not found",
			}
		}

		return article
	})

	b.Handle("PATCH", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request, ctx context.Context) any {
		articleId := box.GetUrlParameter(ctx, "articleId")

		article, exist := articles[articleId]
		if !exist {
			w.WriteHeader(http.StatusNotFound)
			return JSON{
				"error": "article not found",
			}
		}

		err := json.NewDecoder(r.Body).Decode(&article)
		if err != nil {
			return JSON{
				"error": "could not read JSON",
			}
		}

		return article
	})

	b.Handle("DELETE", "/v1/articles/{articleId}", func(w http.ResponseWriter, ctx context.Context) any {

		articleId := box.GetUrlParameter(ctx, "articleId")

		article, exist := articles[articleId]
		if !exist {
			w.WriteHeader(http.StatusNotFound)
			return JSON{
				"error": "article not found",
			}
		}

		delete(articles, articleId)

		return article
	})

	b.Handle("POST", "/v1/articles/{articleId}/publish", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: publish article"
	})

	return b
}
