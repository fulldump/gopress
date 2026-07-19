package api

import (
	"encoding/json"
	"log"
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

func NewApi(staticsDir, version string, db *inceptiondb.Client, fs filestorage.Filestorager, moderator ContentModerator) *box.B {
	if err := db.CreateCollection(commentsCollection); err != nil && err != inceptiondb.ErrorAlreadyExist {
		log.Println("initialize comments collection:", err.Error())
	}

	b := box.NewBox()

	b.WithInterceptors(AccessLog)

	et, err := templates.NewEmbeddedTemplates()
	if err != nil {
		panic(err)
	}
	b.WithInterceptors(templates.Inject(et))

	b.WithInterceptors(PrettyError)
	b.WithInterceptors(InjectInceptionClient(db))
	b.WithInterceptors(InjectContentModerator(moderator))

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

	// API endpoints
	v1 := b.Group("/v1")
	v1.Handle("GET", "/articles/{articleId}/comments", ListComments)
	v1.Handle("POST", "/articles/{articleId}/comments", CreateComment)

	privateEndpoint := func(method, path string, handler any) *box.A {
		return v1.Handle(method, path, handler).WithInterceptors(glueauth.Require)
	}
	privateEndpoint("GET", "/articles", ListArticles)
	privateEndpoint("POST", "/articles", CreateArticle)
	privateEndpoint("GET", "/articles/{articleId}", GetArticle)
	privateEndpoint("PATCH", "/articles/{articleId}", PatchArticle)
	privateEndpoint("DELETE", "/articles/{articleId}", DeleteArticle)
	privateEndpoint("POST", "/articles/{articleId}/publish", PublishArticle)
	privateEndpoint("POST", "/articles/{articleId}/unpublish", UnpublishArticle)
	privateEndpoint("POST", "/files", UploadFile).WithAttribute("filestorager", fs)
	privateEndpoint("GET", "/files", ListFiles)
	privateEndpoint("GET", "/files/{fileId}", RetrieveFile)
	privateEndpoint("DELETE", "/files/{fileId}", DeleteFile)
	privateEndpoint("GET", "/editor/helperFetchUrl", HelperFetchUrl)

	// openapi
	buildOpenApi(b)

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
