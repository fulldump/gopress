package api

import (
	"encoding/json"
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

	et, err := templates.NewEmbeddedTemplates()
	if err != nil {
		panic(err)
	}
	b.WithInterceptors(templates.Inject(et))

	b.WithInterceptors(PrettyError)
	b.WithInterceptors(InjectInceptionClient(db))

	// Public endpoints
	b.Handle("GET", "/", RenderHome)
	b.Handle("GET", "/user/{userNick}", RenderUser)
	b.Handle("GET", "/user/{userNick}/article/{articleUrl}", RenderArticle)
	b.Handle("GET", "/tag/{tag}", RenderTag)
	b.Handle("GET", "/user/{userNick}/tag/{tag}", RenderUserTag)
	b.Handle("GET", "/files/{fileId}", GetFile).WithAttribute("filestorager", fs)
	b.Handle("GET", "/sitemap.xml", Sitemap)
	b.Handle("GET", "/version", func() string {
		return version
	}).WithName("Version")

	// openapi
	buildOpenApi(b)

	// API private endpoints
	v1 := b.Group("/v1").WithInterceptors(glueauth.Require)
	v1.Handle("GET", "/articles", ListArticles)
	v1.Handle("POST", "/articles", CreateArticle)
	v1.Handle("GET", "/articles/{articleId}", GetArticle)
	v1.Handle("PATCH", "/articles/{articleId}", PatchArticle)
	v1.Handle("DELETE", "/articles/{articleId}", DeleteArticle)
	v1.Handle("POST", "/articles/{articleId}/publish", PublishArticle)
	v1.Handle("POST", "/articles/{articleId}/unpublish", UnpublishArticle)
	v1.Handle("POST", "/files", UploadFile).WithAttribute("filestorager", fs)
	v1.Handle("GET", "/files", ListFiles)
	v1.Handle("GET", "/files/{fileId}", RetrieveFile)
	v1.Handle("DELETE", "/files/{fileId}", DeleteFile)
	v1.Handle("GET", "/editor/helperFetchUrl", HelperFetchUrl)

	// Mount statics
	b.Handle("GET", "/*", statics.ServeStatics(staticsDir)).WithName("serveStatics")

	return b
}

func buildOpenApi(b *box.B) {
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
}
