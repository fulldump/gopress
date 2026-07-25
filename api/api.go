package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

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
	for _, collection := range []string{"articles", commentsCollection, articleInteractionsCollection, "files"} {
		if err := db.CreateCollection(collection); err != nil && err != inceptiondb.ErrorAlreadyExist {
			log.Panicf("initialize %s collection: %v", collection, err)
		}
	}
	if err := InitializeBlogCollections(db); err != nil {
		log.Panicf("initialize blog collections: %v", err)
	}
	if err := EnsureBlogIndexes(db); err != nil {
		log.Panicf("initialize blog indexes: %v", err)
	}
	report, err := MigrateBlogs(db)
	if err != nil {
		log.Panicf("blog migration failed safely: %v (report: %+v)", err, report)
	}
	log.Printf("blog migration complete: scanned=%d authors=%d blogs_created=%d blogs_repaired=%d memberships_created=%d articles_patched=%d unchanged=%d skipped=%d",
		report.ArticlesScanned, report.AuthorsFound, report.BlogsCreated, report.BlogsRepaired,
		report.MembershipsCreated, report.ArticlesPatched, report.ArticlesUnchanged, report.ArticlesSkipped)
	logMigrationWarnings("article", report.Errors)
	banReport, err := ReconcileBannedBlogs(db, time.Time{})
	if err != nil {
		log.Panicf("initial blog ban reconciliation failed: %v", err)
	}
	logBlogBanReport("initial", banReport)
	fileReport, err := MigrateFileBlogIds(db)
	if err != nil {
		log.Panicf("file blog migration failed safely: %v (report: %+v)", err, fileReport)
	}
	if fileReport.FilesScanned > 0 {
		log.Printf("file blog migration complete: scanned=%d blogs_created=%d files_patched=%d unchanged=%d skipped=%d",
			fileReport.FilesScanned, fileReport.BlogsCreated, fileReport.FilesPatched, fileReport.FilesUnchanged, fileReport.FilesSkipped)
	}
	logMigrationWarnings("file", fileReport.Errors)
	commentVersions, err := MigrateCommentArticleVersions(db)
	if err != nil {
		log.Panicf("comment article-version migration failed: %v", err)
	}
	if commentVersions > 0 {
		log.Printf("comment migration complete: patched=%d", commentVersions)
	}
	if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
		Name:   "interaction-user-article-version-type",
		Type:   "btree",
		Fields: []string{"userId", "articleId", "articleCreatedOn", "type"},
		Unique: true,
	}); err != nil {
		log.Panicln("initialize article interactions index:", err.Error())
	}
	if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
		Name:   "interactions-by-article-version",
		Type:   "btree",
		Fields: []string{"articleId", "articleCreatedOn", "id"},
		Unique: true,
	}); err != nil {
		log.Panicln("initialize article interactions article index:", err.Error())
	}
	if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
		Name:   "interactions-by-user-type",
		Type:   "btree",
		Fields: []string{"userId", "type", "id"},
		Unique: true,
	}); err != nil {
		log.Panicln("initialize article interactions user index:", err.Error())
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
	b.Handle("GET", "/blogs", RenderBlogs)
	b.Handle("GET", "/blogs/", RedirectBlogsSlash)
	b.Handle("GET", "/blog/{blogSlug}", RenderBlog)
	b.Handle("GET", "/blog/{blogSlug}/article/{articleUrl}", RenderBlogArticle)
	b.Handle("GET", "/blog/{blogSlug}/tag/{tag}", RenderBlogTag)
	b.Handle("GET", "/user/{userNick}", RenderUser)
	b.Handle("GET", "/user/{userNick}/article/{articleUrl}", RenderArticle)
	b.Handle("GET", "/tag/{tag}", RenderTag)
	b.Handle("GET", "/user/{userNick}/tag/{tag}", RenderUserTag)
	b.Handle("GET", "/bookmarks", RenderBookmarks)
	b.Handle("GET", "/files/{fileId}", GetFile).WithInterceptors(glueauth.Auth).WithAttribute("filestorager", fs)
	b.Handle("GET", "/sitemap.xml", Sitemap)
	b.Handle("GET", "/version", func() string {
		return version
	}).WithName("Version")

	// API endpoints
	v1 := b.Group("/v1")
	v1.Handle("GET", "/articles/{articleId}/comments", ListComments)
	v1.Handle("POST", "/articles/{articleId}/comments", CreateComment)
	v1.Handle("GET", "/articles/{articleId}/feedback", GetArticleFeedback).WithInterceptors(glueauth.Auth)

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
	privateEndpoint("POST", "/articles/{articleId}/edit-lock/acquire", AcquireArticleLock)
	privateEndpoint("POST", "/articles/{articleId}/edit-lock/renew", RenewArticleLock)
	privateEndpoint("POST", "/articles/{articleId}/edit-lock/release", ReleaseArticleLock)
	privateEndpoint("GET", "/blogs", ListBlogs)
	privateEndpoint("POST", "/blogs", CreateBlog)
	privateEndpoint("GET", "/blogs/{blogId}", GetBlog)
	privateEndpoint("PATCH", "/blogs/{blogId}", PatchBlog)
	privateEndpoint("GET", "/blogs/{blogId}/members", ListBlogMembers)
	privateEndpoint("DELETE", "/blogs/{blogId}/members/{userId}", RemoveBlogMember)
	privateEndpoint("GET", "/blogs/{blogId}/invitations", ListBlogInvitations)
	privateEndpoint("POST", "/blogs/{blogId}/invitations", CreateBlogInvitation)
	privateEndpoint("DELETE", "/blogs/{blogId}/invitations/{invitationId}", RevokeBlogInvitation)
	privateEndpoint("POST", "/blogs/{blogId}/review-request", RequestBlogReview)
	privateEndpoint("POST", "/invitations/accept", AcceptBlogInvitation)
	privateEndpoint("GET", "/blogs/{blogId}/articles", ListArticles)
	privateEndpoint("POST", "/blogs/{blogId}/articles", CreateArticle)
	privateEndpoint("GET", "/blogs/{blogId}/articles/{articleId}", GetArticle)
	privateEndpoint("PATCH", "/blogs/{blogId}/articles/{articleId}", PatchArticle)
	privateEndpoint("DELETE", "/blogs/{blogId}/articles/{articleId}", DeleteArticle)
	privateEndpoint("POST", "/blogs/{blogId}/articles/{articleId}/publish", PublishArticle)
	privateEndpoint("POST", "/blogs/{blogId}/articles/{articleId}/unpublish", UnpublishArticle)
	privateEndpoint("POST", "/blogs/{blogId}/articles/{articleId}/edit-lock/acquire", AcquireArticleLock)
	privateEndpoint("POST", "/blogs/{blogId}/articles/{articleId}/edit-lock/renew", RenewArticleLock)
	privateEndpoint("POST", "/blogs/{blogId}/articles/{articleId}/edit-lock/release", ReleaseArticleLock)
	privateEndpoint("PUT", "/articles/{articleId}/like", LikeArticle)
	privateEndpoint("DELETE", "/articles/{articleId}/like", UnlikeArticle)
	privateEndpoint("PUT", "/articles/{articleId}/bookmark", BookmarkArticle)
	privateEndpoint("DELETE", "/articles/{articleId}/bookmark", RemoveArticleBookmark)
	privateEndpoint("GET", "/bookmarks", ListBookmarks)
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

func logMigrationWarnings(kind string, warnings []string) {
	const maxWarnings = 10
	for index, warning := range warnings {
		if index == maxWarnings {
			log.Printf("%s migration: %d additional warnings omitted", kind, len(warnings)-maxWarnings)
			break
		}
		log.Printf("%s migration warning: %s", kind, warning)
	}
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
