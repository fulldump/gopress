package api

import (
	"context"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/inceptiondb"
	"gopress/statics"
	"gopress/templates"
)

type Article struct {
	Id      string `json:"id"` // todo: this is part of persistence layer/logic
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

func NewApi(x map[string]*Article, staticsDir string, db *inceptiondb.Client) *box.B {

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

	b.WithInterceptors(InjectInceptionClient(db))

	templateHome, err := template.New("").Parse(templates.Home)
	if err != nil {
		panic(err) // todo: handle this properly
	}

	b.Handle("GET", "/", func(w http.ResponseWriter) {
		// todo: limit page size to 10
		// todo: sort by date DESC

		list := map[string]*Article{}
		db.FindAll("articles", inceptiondb.FindQuery{Limit: 1000}, func(article *Article) {
			list[article.Id] = article
			//			list = append(list, article)
		}) // todo: handle error properly

		err := templateHome.ExecuteTemplate(w, "", map[string]any{
			"articles": list,
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

		article := &Article{}
		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id": articleId,
			},
		}, article)
		if err != nil {
			log.Println("render article: db find:", err.Error())
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Article not found"))
			return
		}

		err = templateArticle.ExecuteTemplate(w, "", map[string]any{
			"article": article,
		})

		if err != nil {
			log.Println("Error rendering home:", err.Error())
		}
	}).WithName("RenderArticle")

	b.Handle("GET", "/v1/articles", func() any {

		result := []*ArticleShort{}

		db.FindAll("articles", inceptiondb.FindQuery{Limit: 1000}, func(article *Article) {
			result = append(result, &ArticleShort{
				Id:    article.Id,
				Title: article.Title,
			})
		})

		return result
	}).WithName("ListArticles")

	type CreateArticleRequest struct {
		Id    string `json:"id"`
		Title string `json:"title"`
	}

	b.Handle("POST", "/v1/articles", func(input *CreateArticleRequest) any {

		if input.Id == "" {
			input.Id = uuid.New().String()
		}

		newArticle := &Article{
			Id:        input.Id,
			Title:     input.Title,
			Content:   "Start here",
			CreatedOn: time.Now(),
			Published: false,
		}

		err := db.Insert("articles", newArticle)
		if err != nil {
			log.Println("create article: insert:", err.Error())
			return JSON{
				"error": "error creating article",
			}
		}

		return newArticle
	}).WithName("CreateArticle")

	b.Handle("GET", "/v1/articles/{articleId}", func(ctx context.Context, w http.ResponseWriter) any {

		articleId := box.GetUrlParameter(ctx, "articleId")

		article := &Article{}
		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id": articleId,
			},
		}, article)
		if err != nil {
			log.Println("render article: db find:", err.Error())
			w.WriteHeader(http.StatusNotFound)
			return "Article not found"
		}

		return article
	}).WithName("GetArticle")

	b.Handle("PATCH", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request, ctx context.Context) any {
		articleId := box.GetUrlParameter(ctx, "articleId")

		article := &Article{}

		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id": articleId,
			},
		}, article)
		if err != nil {
			log.Println("patch article: db find:", err.Error())
			return JSON{
				"error": "could not read from data storage",
			}
		}

		err = json.NewDecoder(r.Body).Decode(&article)
		if err != nil {
			log.Println("patch article: json decode:", err.Error())
			return JSON{
				"error": "could not read JSON",
			}
		}

		_, err = db.Patch("articles", inceptiondb.PatchQuery{
			Filter: JSON{
				"id": articleId,
			},
			Patch: article,
		})
		if err != nil {
			log.Println("patch article: db patch:", err.Error())
			return JSON{
				"error": "could not write to data storage",
			}
		}

		return article
	}).WithName("PatchArticle")

	b.Handle("DELETE", "/v1/articles/{articleId}", func(w http.ResponseWriter, ctx context.Context) any {

		articleId := box.GetUrlParameter(ctx, "articleId")

		r, err := db.Remove("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id": articleId,
			},
		})
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return JSON{
				"error": "article not found",
			}
		}

		// todo: handle errors properly
		io.Copy(w, r)
		r.Close()

		return nil
	}).WithName("DeleteArticle")

	b.Handle("POST", "/v1/articles/{articleId}/publish", func(w http.ResponseWriter, r *http.Request) string {
		return "todo: publish article"
	}).WithName("PublishArticle")

	// Mount statics
	b.Handle("GET", "/*", statics.ServeStatics(staticsDir)).WithName("serveStatics")

	return b
}
