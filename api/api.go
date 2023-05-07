package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/glueauth"
	"gopress/inceptiondb"
	"gopress/statics"
	"gopress/templates"
)

type Article struct {
	Id string `json:"id"` // todo: this is part of persistence layer/logic

	ArticleUserFields

	CreatedOn     time.Time `json:"createdOn"`
	PublishOn     time.Time `json:"publishOn"`
	Published     bool      `json:"published"` // todo: use date to program publishment in the future
	AuthorId      string    `json:"author_id"`
	AuthorNick    string    `json:"author_nick"`
	AuthorPicture string    `json:"author_picture"`

	Stats ArticleStats `json:"stats"`
}

type ArticleStats struct {
	Views uint64 `json:"views"` // total number of views
	//	Impressions uint64 `json:"impressions"` // total number of impressions
}

type ArticleUserFields struct {
	Url     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type ArticleShort struct {
	Id        string       `json:"id"`
	Title     string       `json:"title"`
	Url       string       `json:"url"`
	Published bool         `json:"published"`
	Stats     ArticleStats `json:"stats"`
}

type JSON map[string]any

func NewApi(staticsDir string, db *inceptiondb.Client) *box.B {

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

		query := inceptiondb.FindQuery{
			Limit: 1000,
			Filter: JSON{
				"published": true,
			},
		}

		list := map[string]*Article{}
		db.FindAll("articles", query, func(article *Article) {
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

	b.Handle("GET", "/articles/{articleUrl}", func(w http.ResponseWriter, ctx context.Context) {

		articleUrl := box.GetUrlParameter(ctx, "articleUrl")

		filter := JSON{
			"url":       articleUrl,
			"published": true,
		}

		article := &Article{}
		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: filter,
		}, article)
		if err != nil {
			log.Println("render article: db find:", err.Error())
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Article not found"))
			return
		}

		words := strings.SplitN(article.Content, " ", 10)
		title := "@" + article.AuthorNick + ": " + strings.Join(words[0:len(words)-1], " ") + "..."
		selfUrl := `https://gopress.org/user/` + url.PathEscape(article.AuthorId)

		description := article.Content
		max_description := 150
		if len(description) > max_description {
			description = article.Content[0:max_description] + "..."
		}

		err = templateArticle.ExecuteTemplate(w, "", map[string]any{
			"article": article,

			"og_title":       title,
			"og_url":         selfUrl,
			"og_image":       article.AuthorPicture,
			"og_description": description,
		})

		if err != nil {
			log.Println("Error rendering home:", err.Error())
			return
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Println("RenderArticle: db patch:", r)
				}
			}()
			db.Patch("articles", inceptiondb.PatchQuery{
				Filter: filter,
				Patch: JSON{
					"stats": JSON{
						"views": article.Stats.Views + 1,
					},
				},
			})
		}()

	}).WithName("RenderArticle")

	templateUser, err := template.New("").Parse(templates.User)
	if err != nil {
		panic(err) // todo: handle this properly
	}

	b.Handle("GET", "/user/{userId}", func(w http.ResponseWriter, ctx context.Context) {
		// todo: limit page size to 10
		// todo: sort by date DESC

		userId := box.GetUrlParameter(ctx, "userId")

		userNick := userId
		params := inceptiondb.FindQuery{
			Limit: 1000,
			Filter: JSON{
				"author_id": userId,
				"published": true,
			},
		}
		list := map[string]*Article{}
		db.FindAll("articles", params, func(article *Article) {
			list[article.Id] = article
			userNick = article.AuthorNick
		}) // todo: handle error properly
		if len(list) == 0 {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("User not found"))
			return
		}

		err := templateUser.ExecuteTemplate(w, "", map[string]any{
			"userId":   userId,
			"userNick": userNick,
			"articles": list,
		})

		if err != nil {
			log.Println("Error rendering home:", err.Error())
		}
	}).WithName("RenderHome")

	b.Handle("GET", "/sitemap.xml", func(w http.ResponseWriter) {

		w.Header().Set("content-type", "text/xml; charset=UTF-8")
		// Begin XML
		w.Write([]byte(xml.Header))
		w.Write([]byte(`<urlset xmlns="http://www.google.com/schemas/sitemap/0.9">` + "\n"))

		// Collect users
		users := map[string]*Article{}

		// Article pages
		params := inceptiondb.FindQuery{
			Limit: 9999,
		}
		db.FindAll("articles", params, func(article *Article) {
			w.Write([]byte(`    <url>
        <loc>https://gopress.org/articles/` + article.Url + `</loc>
        <lastmod>` + article.CreatedOn.Format("2006-01-02") + `</lastmod>
        <changefreq>weekly</changefreq>
        <priority>0.4</priority>
    </url>`))

			lastArticle, exist := users[article.AuthorId]
			if !exist {
				users[article.AuthorId] = article
			} else if article.CreatedOn.UnixNano() > lastArticle.CreatedOn.UnixNano() {
				users[article.AuthorId] = article
			}

		})

		// User pages
		for userId, article := range users {
			w.Write([]byte(`    <url>
        <loc>https://gopress.org/user/` + userId + `</loc>
        <lastmod>` + article.CreatedOn.Format("2006-01-02") + `</lastmod>
        <changefreq>weekly</changefreq>
        <priority>0.4</priority>
    </url>`))
		}

		// End XML
		w.Write([]byte(`</urlset>`))

	}).WithName("Sitemap")

	b.Group("/v1").WithInterceptors(glueauth.Require)

	b.Handle("GET", "/v1/articles", func(ctx context.Context) any {

		auth := glueauth.GetAuth(ctx)
		query := inceptiondb.FindQuery{
			Limit: 1000,
			Filter: JSON{
				"author_id": auth.User.ID,
			},
		}

		result := []*ArticleShort{}
		db.FindAll("articles", query, func(article *Article) {
			result = append(result, &ArticleShort{
				Id:        article.Id,
				Title:     article.Title,
				Url:       article.Url,
				Published: article.Published,
				Stats:     article.Stats,
			})
		})

		return result
	}).WithName("ListArticles")

	type CreateArticleRequest struct {
		Id    string `json:"id"`
		Title string `json:"title"`
	}

	b.Handle("POST", "/v1/articles", func(input *CreateArticleRequest, ctx context.Context) any {

		if input.Id == "" {
			input.Id = uuid.New().String()
		}

		auth := glueauth.GetAuth(ctx)

		newArticle := &Article{
			Id:            input.Id,
			AuthorId:      auth.User.ID,
			AuthorNick:    auth.User.Nick,
			AuthorPicture: auth.User.Picture,
			CreatedOn:     time.Now(),
			Published:     false,
			ArticleUserFields: ArticleUserFields{
				Title:   input.Title,
				Url:     Slug(input.Title) + "-" + uuid.New().String(),
				Content: "Start here",
			},
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

		auth := glueauth.GetAuth(ctx)

		article := &Article{}
		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id":        articleId,
				"author_id": auth.User.ID,
			},
		}, article)
		if err != nil {
			log.Println("render article: db find:", err.Error())
			w.WriteHeader(http.StatusNotFound)
			return "Article not found"
		}

		return article
	}).WithName("GetArticle")

	type PatchInput struct {
		Url     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	b.Handle("PATCH", "/v1/articles/{articleId}", func(w http.ResponseWriter, r *http.Request, ctx context.Context) any {
		articleId := box.GetUrlParameter(ctx, "articleId")

		auth := glueauth.GetAuth(ctx)

		article := &Article{}

		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id":        articleId,
				"author_id": auth.User.ID,
			},
		}, article)
		if err != nil {
			log.Println("patch article: db find:", err.Error())
			return JSON{
				"error": "could not read from data storage",
			}
		}

		err = json.NewDecoder(r.Body).Decode(&article.ArticleUserFields)
		if err != nil {
			log.Println("patch article: json decode:", err.Error())
			return JSON{
				"error": "could not read JSON",
			}
		}

		_, err = db.Patch("articles", inceptiondb.PatchQuery{
			Filter: JSON{
				"id":        articleId,
				"author_id": auth.User.ID,
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

		auth := glueauth.GetAuth(ctx)

		r, err := db.Remove("articles", inceptiondb.FindQuery{
			Filter: JSON{
				"id":        articleId,
				"author_id": auth.User.ID,
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

	b.Handle("POST", "/v1/articles/{articleId}/publish", func(w http.ResponseWriter, r *http.Request, ctx context.Context) any {

		articleId := box.GetUrlParameter(ctx, "articleId")
		auth := glueauth.GetAuth(ctx)

		filter := JSON{
			"id":        articleId,
			"author_id": auth.User.ID,
		}

		article := &Article{}
		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: filter,
		}, article)
		if err != nil {
			log.Println("publish article: db find:", err.Error())
			return JSON{
				"error": "could not read from data storage",
			}
		}

		article.Published = true
		article.PublishOn = time.Now()

		_, err = db.Patch("articles", inceptiondb.PatchQuery{
			Filter: filter,
			Patch:  article,
		})
		if err != nil {
			log.Println("publish article: db patch:", err.Error())
			return JSON{
				"error": "could not write to data storage",
			}
		}

		return article

	}).WithName("PublishArticle")

	b.Handle("POST", "/v1/articles/{articleId}/unpublish", func(w http.ResponseWriter, r *http.Request, ctx context.Context) any {

		articleId := box.GetUrlParameter(ctx, "articleId")
		auth := glueauth.GetAuth(ctx)

		filter := JSON{
			"id":        articleId,
			"author_id": auth.User.ID,
		}

		article := &Article{}
		err := db.FindOne("articles", inceptiondb.FindQuery{
			Filter: filter,
		}, article)
		if err != nil {
			log.Println("unpublish article: db find:", err.Error())
			return JSON{
				"error": "could not read from data storage",
			}
		}

		article.Published = false

		_, err = db.Patch("articles", inceptiondb.PatchQuery{
			Filter: filter,
			Patch:  article,
		})
		if err != nil {
			log.Println("unpublish article: db patch:", err.Error())
			return JSON{
				"error": "could not write to data storage",
			}
		}

		return article

	}).WithName("UnpublishArticle")

	// Mount statics
	b.Handle("GET", "/*", statics.ServeStatics(staticsDir)).WithName("serveStatics")

	return b
}

func Slug(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")

	return s
}
