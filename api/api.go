package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fulldump/box"
	"github.com/google/uuid"

	"gopress/filestorage"
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
	Url         string        `json:"url"`
	Title       string        `json:"title"`
	Content     Content       `json:"content"`
	ContentHTML template.HTML `json:"content_html"` // it works like a cache
	Tags        []string      `json:"tags"`
}

type Content struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ArticleShort struct {
	Id        string       `json:"id"`
	Title     string       `json:"title"`
	Url       string       `json:"url"`
	Published bool         `json:"published"`
	Stats     ArticleStats `json:"stats"`
	Tags      []string     `json:"tags"`
}

type File struct {
	Id string `json:"id"` // todo: this is part of persistence layer/logic

	AuthorId      string `json:"author_id"`
	AuthorNick    string `json:"author_nick"`
	AuthorPicture string `json:"author_picture"`

	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`

	CreatedOn time.Time `json:"createdOn"`
}

type JSON map[string]any

func NewApi(staticsDir string, db *inceptiondb.Client, fs filestorage.Filestorager) *box.B {

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

		// TODO: preprocess html tags to remove

		words := strings.SplitN(string(article.ContentHTML), " ", 10)
		title := "@" + article.AuthorNick + ": " + strings.Join(words[0:len(words)-1], " ") + "..."
		selfUrl := `https://gopress.org/user/` + url.PathEscape(article.AuthorId)

		description := article.ContentHTML
		max_description := 150
		if len(description) > max_description {
			description = article.ContentHTML[0:max_description] + "..."
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

	templateTag, err := template.New("").Parse(templates.Tag)
	if err != nil {
		panic(err) // todo: handle this properly
	}

	b.Handle("GET", "/tag/{tag}", func(w http.ResponseWriter, ctx context.Context) {
		// todo: limit page size to 10
		// todo: sort by date DESC

		tag := box.GetUrlParameter(ctx, "tag")

		params := inceptiondb.FindQuery{
			Limit: 1000,
			Filter: JSON{
				"tags":      tag,
				"published": true,
			},
		}
		list := map[string]*Article{}
		db.FindAll("articles", params, func(article *Article) {
			list[article.Id] = article
		}) // todo: handle error properly
		if len(list) == 0 {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("User not found"))
			return
		}

		err := templateTag.ExecuteTemplate(w, "", map[string]any{
			"tag":      tag,
			"articles": list,
		})

		if err != nil {
			log.Println("Error rendering home:", err.Error())
		}
	}).WithName("RenderHome")

	b.Handle("GET", "/user/{userId}/tag/{tag}", func(w http.ResponseWriter, ctx context.Context) {
		// todo: limit page size to 10
		// todo: sort by date DESC

		userId := box.GetUrlParameter(ctx, "userId")
		tag := box.GetUrlParameter(ctx, "tag")
		userNick := userId

		params := inceptiondb.FindQuery{
			Limit: 1000,
			Filter: JSON{
				"author_id": userId,
				"tags":      tag,
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
			"tag":      tag,
			"userId":   userId,
			"userNick": userNick,
			"articles": list,
		})

		if err != nil {
			log.Println("Error rendering home:", err.Error())
		}
	}).WithName("RenderHome")

	b.Handle("GET", "/files/{fileId}", func(w http.ResponseWriter, ctx context.Context) error {

		fileId := box.GetUrlParameter(ctx, "fileId")

		file := &File{}

		err := db.FindOne("files", inceptiondb.FindQuery{Filter: JSON{"id": fileId}}, file)
		if err != nil {
			log.Println(err.Error())
			return errors.New("file not found")
		}

		r, err := fs.OpenReader(fileId)
		if err != nil {
			log.Println(err.Error())
			return errors.New("file not found")
		}

		w.Header().Set("Content-Type", file.Mime) // todo: only if not empty?

		io.Copy(w, r) // todo: handle error properly

		return nil
	})

	b.Handle("GET", "/sitemap.xml", func(w http.ResponseWriter) {

		w.Header().Set("content-type", "text/xml; charset=UTF-8")
		// Begin XML
		w.Write([]byte(xml.Header))
		w.Write([]byte(`<urlset xmlns="http://www.google.com/schemas/sitemap/0.9">` + "\n"))

		// Collect users
		users := map[string]*Article{}
		tags := map[string]*Article{}

		// Article pages
		params := inceptiondb.FindQuery{
			Limit: 9999,
			Filter: JSON{
				"published": true,
			},
		}
		db.FindAll("articles", params, func(article *Article) {
			w.Write([]byte(`    <url>
        <loc>https://gopress.org/articles/` + article.Url + `</loc>
        <lastmod>` + article.CreatedOn.UTC().Format("2006-01-02") + `</lastmod>
        <changefreq>weekly</changefreq>
        <priority>0.6</priority>
    </url>`))

			{
				lastArticle, exist := users[article.AuthorId]
				if !exist || article.CreatedOn.After(lastArticle.CreatedOn) {
					users[article.AuthorId] = article
				}
			}

			for _, tag := range article.Tags {
				lastArticle, exist := tags[tag]
				if !exist || article.CreatedOn.After(lastArticle.CreatedOn) {
					tags[tag] = article
				}
			}
		})

		// User pages
		for userId, article := range users {
			w.Write([]byte(`    <url>
        <loc>https://gopress.org/user/` + userId + `</loc>
        <lastmod>` + article.CreatedOn.UTC().Format("2006-01-02") + `</lastmod>
        <changefreq>daily</changefreq>
        <priority>0.4</priority>
    </url>`))
		}

		// Tag pages
		for tag, lastArticle := range tags {
			w.Write([]byte(`    <url>
        <loc>https://gopress.org/tag/` + tag + `</loc>
        <lastmod>` + lastArticle.CreatedOn.UTC().Format("2006-01-02") + `</lastmod>
        <changefreq>hourly</changefreq>
        <priority>0.2</priority>
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
				Tags:      article.Tags,
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
			input.Id = "article_" + uuid.New().String()
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
				Title: input.Title,
				Url:   Slug(input.Title) + "-" + uuid.New().String(),
				// Content: // todo: some default value?,
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

		contentHtml, err := editorjs2HTML(article.Content.Data)
		if err != nil {
			return JSON{
				"error": "invalid payload to transform from editorjs 2 html",
			}
		}
		article.ContentHTML = template.HTML(contentHtml)

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

	type UploadFileOutput struct {
		Files []*File `json:"files"`
	}

	var ErrorUploadFilesMultipart = errors.New("multipart method is required")
	var maxUploadBytes = int64(25 * 1024 * 1024)
	var ErrorMaxUploadSize = errors.New(fmt.Sprintf("file size should be less than %d bytes", maxUploadBytes))

	var ErrorPersistenceWrite = errors.New("unexpected internal error writing to persistence layer")
	var ErrorPersistenceRead = errors.New("unexpected internal error reading from persistence layer")

	b.Handle("POST", "/v1/files", func(w http.ResponseWriter, r *http.Request, ctx context.Context) (any, error) {

		auth := glueauth.GetAuth(ctx)

		response := &UploadFileOutput{
			Files: []*File{},
		}

		m, err := r.MultipartReader()
		if err != nil {
			log.Println(err.Error())
			return nil, ErrorUploadFilesMultipart
		}

		for {
			part, err := m.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Println(err.Error())
				break // todo: previously was continue (too risky?)
			}

			name := part.FormName()
			mime := part.Header.Get("Content-Type")
			log.Printf("Name: %s; Mime: %s", name, mime)

			fileId := "file_" + uuid.New().String()

			w, err := fs.OpenWriter(fileId)
			if err != nil {
				log.Println(err.Error())
				return nil, ErrorPersistenceWrite
			}

			n, err := copyMaxBytes(w, part, maxUploadBytes)
			if err != nil {
				log.Println(err.Error())
				return nil, ErrorPersistenceWrite
			}
			if n == maxUploadBytes {
				log.Println(ErrorMaxUploadSize)
				return nil, ErrorMaxUploadSize
			}

			now := time.Now().UTC()

			file := &File{
				Id:            fileId,
				AuthorId:      auth.User.ID,
				AuthorNick:    auth.User.Nick,
				AuthorPicture: auth.User.Picture,
				Name:          name,
				Size:          n,
				Mime:          mime,
				CreatedOn:     now,
			}
			response.Files = append(response.Files, file)

			err = db.Insert("files", file)
			if err != nil {
				log.Println(err.Error())
				return nil, ErrorPersistenceWrite
			}

		}

		w.WriteHeader(http.StatusCreated)

		// return response, nil

		return JSON{

			"success": 1,
			"file": JSON{
				"url": "http://localhost:9955/files/" + response.Files[0].Id,
			},
		}, nil

	}).WithName("UploadFile")

	b.Handle("GET", "/v1/files", func(w http.ResponseWriter, r *http.Request, ctx context.Context) ([]*File, error) {

		auth := glueauth.GetAuth(ctx)

		response := []*File{}

		query := inceptiondb.FindQuery{
			Limit: 1000,
			Filter: JSON{
				"author_id": auth.User.ID,
			},
		}

		db.FindAll("files", query, func(file *File) {
			response = append(response, file)
		})

		return response, nil
	}).WithName("ListFiles")

	b.Handle("GET", "/v1/files/{{fileId}}", func(w http.ResponseWriter, r *http.Request, ctx context.Context) (*File, error) {

		fileId := box.GetUrlParameter(ctx, "fileId")
		auth := glueauth.GetAuth(ctx)

		query := inceptiondb.FindQuery{
			Limit: 1000,
			Filter: JSON{
				"id":        fileId,
				"author_id": auth.User.ID,
			},
		}

		response := &File{}

		err := db.FindOne("files", query, response)
		if err != nil {
			log.Println(err.Error())
			return nil, ErrorPersistenceRead
		}

		return response, nil
	}).WithName("RetrieveFile")

	b.Handle("DELETE", "/v1/files/{{fileId}}", func(w http.ResponseWriter, r *http.Request, ctx context.Context) (*File, error) {

		// TODO: implement this!!!

		return nil, nil
	}).WithName("DeleteFile")

	// Mount statics
	b.Handle("GET", "/*", statics.ServeStatics(staticsDir)).WithName("serveStatics")

	return b
}

func Slug(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")

	return s
}

func copyMaxBytes(w io.WriteCloser, r io.ReadCloser, max int64) (int64, error) {

	n, err := io.CopyN(w, r, max)
	if err == io.EOF {
		// All is OK
	} else if err != nil {
		return n, err
	}

	err = w.Close()
	if err != nil {
		return 0, err
	}

	return n, nil
}

func editorjs2HTML(data []byte) (string, error) {

	e := &EditorJs{}
	err := json.Unmarshal(data, &e)
	if err != nil {
		return "", err
	}

	result := &strings.Builder{}

	for _, block := range e.Blocks {

		switch block.Type {
		case "header":
			header := struct {
				Level int
				Text  string
			}{}
			json.Unmarshal(block.Data, &header) // todo: handle error properly
			fmt.Fprintf(result, "<h%d>%s</h%d>\n", header.Level, header.Text, header.Level)
		case "paragraph":
			header := struct {
				Text string
			}{}
			json.Unmarshal(block.Data, &header) // todo: handle error properly
			fmt.Fprintf(result, "<p>%s</p>\n", header.Text)
		case "image":
			// todo
		case "list":
			list := struct {
				Style string
				Items []string
			}{}
			json.Unmarshal(block.Data, &list) // todo: handle error properly
			tag := "ul"
			if list.Style == "ordered" {
				tag = "ol"
			}
			fmt.Fprintf(result, "<%s>\n", tag)
			for _, item := range list.Items {
				fmt.Fprintf(result, "<li>%s</li>\n", item)
			}
			fmt.Fprintf(result, "</%s>\n", tag)
		case "checklist":
			// todo
		case "quote":
			// todo
		case "warning":
			// todo
		case "delimiter":
			// todo
		case "linkTool":
			// todo
		case "table":
			// todo
		case "raw":
			// todo
		case "attaches":
			// todo
		case "embed":
			embed := struct {
				Caption string
				Embed   string
				Height  int
				Service string
				Source  string
				Width   int
			}{}
			json.Unmarshal(block.Data, &embed) // todo: handle error properly
			fmt.Fprintf(result, `<iframe style="width:100%%;" height="%d" frameborder="0" allowfullscreen="" src="%s" class="embed-tool__content"></iframe>`+"\n",

				embed.Height, embed.Embed)

		}
	}

	return result.String(), nil
}

type EditorJs struct {
	Blocks []*Block `json:"blocks"`
	// time
	// version
}

type Block struct {
	Id   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
