package api

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/fulldump/box"
	"github.com/fulldump/box/boxopenapi"

	"gopress/filestorage"
	"gopress/glueauth"
	"gopress/inceptiondb"
	"gopress/statics"
	"gopress/templates"
)

type JSON = map[string]any

func NewApi(staticsDir, version string, db *inceptiondb.Client, fs filestorage.Filestorager) *box.B {

	b := box.NewBox()

	b.WithInterceptors(AccessLog)
	b.WithInterceptors(func(next box.H) box.H {
		return func(ctx context.Context) {
			next(ctx)

			err := box.GetError(ctx)
			if err != nil {
				json.NewEncoder(box.GetResponse(ctx)).Encode(JSON{
					"error": err.Error(),
				})
			}
		}
	})
	b.WithInterceptors(InjectInceptionClient(db))

	// TODO: move all templates wiring to a helper to manage templates and possibly with an interface abstraction

	templateHome, err := template.New("").Parse(templates.Home)
	if err != nil {
		panic(err) // todo: handle this properly
	}
	b.Handle("GET", "/", RenderHome).WithAttribute("template", templateHome)

	templateUser, err := template.New("").Parse(templates.User)
	if err != nil {
		panic(err) // todo: handle this properly
	}
	b.Handle("GET", "/user/{userNick}", RenderUser).WithAttribute("template", templateUser)

	templateArticle, err := template.New("").Parse(templates.Article)
	if err != nil {
		panic(err) // todo: handle this properly
	}
	b.Handle("GET", "/user/{userNick}/article/{articleUrl}", RenderArticle).WithAttribute("template", templateArticle)

	templateTag, err := template.New("").Parse(templates.Tag)
	if err != nil {
		panic(err) // todo: handle this properly
	}
	b.Handle("GET", "/tag/{tag}", RenderTag).WithAttribute("template", templateTag)

	b.Handle("GET", "/user/{userNick}/tag/{tag}", RenderUserTag).WithAttribute("template", templateUser)

	b.Handle("GET", "/files/{fileId}", GetFile).WithAttribute("filestorager", fs)

	b.Handle("GET", "/sitemap.xml", Sitemap)

	b.Group("/v1").WithInterceptors(glueauth.Require)
	b.Handle("GET", "/v1/articles", ListArticles)
	b.Handle("POST", "/v1/articles", CreateArticle)
	b.Handle("GET", "/v1/articles/{articleId}", GetArticle)
	b.Handle("PATCH", "/v1/articles/{articleId}", PatchArticle)
	b.Handle("DELETE", "/v1/articles/{articleId}", DeleteArticle)
	b.Handle("POST", "/v1/articles/{articleId}/publish", PublishArticle)
	b.Handle("POST", "/v1/articles/{articleId}/unpublish", UnpublishArticle)

	b.Handle("POST", "/v1/files", UploadFile).WithAttribute("filestorager", fs)
	b.Handle("GET", "/v1/files", ListFiles)
	b.Handle("GET", "/v1/files/{{fileId}}", RetrieveFile) // TODO: delete double curly braces
	b.Handle("DELETE", "/v1/files/{{fileId}}", DeleteFile)

	b.Handle("GET", "/editor/helperFetchUrl", HelperFetchUrl).WithName("helperFetchUrl")

	// version
	b.Handle("GET", "/version", func() string {
		return version
	}).WithName("Version")

	// openapi
	spec := boxopenapi.Spec(b)
	spec.Info.Title = "GoPress"
	spec.Info.Description = "A free blogging system in go"
	spec.Info.Contact = &boxopenapi.Contact{
		Url: "https://github.com/fulldump/gopress/issues/new",
	}
	b.Handle("GET", "/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		spec.Servers = []boxopenapi.Server{
			{
				Url: "https://" + r.Host,
			},
			{
				Url: "http://" + r.Host,
			},
		}

		e := json.NewEncoder(w)
		e.SetIndent("", "    ")
		e.Encode(spec)
	}).WithName("OpenApi")

	// Mount statics
	b.Handle("GET", "/*", statics.ServeStatics(staticsDir)).WithName("serveStatics")

	return b
}
