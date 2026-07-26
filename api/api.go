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
	for _, collection := range []string{"articles", commentsCollection, articleInteractionsCollection, "files", userPreferencesCollection} {
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
	localizedArticles, err := MigrateArticleLocalization(db)
	if err != nil {
		log.Panicf("article localization migration failed: %v", err)
	}
	if localizedArticles > 0 {
		log.Printf("article localization migration complete: patched=%d", localizedArticles)
	}
	if err := EnsureArticlePaginationIndexes(db); err != nil {
		log.Panicf("initialize article pagination indexes: %v", err)
	}
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
	memberReport, err := MigrateEmbeddedBlogMembers(db)
	if err != nil {
		log.Panicf("blog member migration failed safely: %v (report: %+v)", err, memberReport)
	}
	log.Printf("blog member migration complete: legacy_collection=%t legacy_rows=%d editors=%d blogs_patched=%d",
		memberReport.LegacyCollectionFound, memberReport.LegacyRowsScanned, memberReport.EditorsMigrated, memberReport.BlogsPatched)
	commentVersions, err := MigrateCommentArticleVersions(db)
	if err != nil {
		log.Panicf("comment article-version migration failed: %v", err)
	}
	if commentVersions > 0 {
		log.Printf("comment migration complete: patched=%d", commentVersions)
	}
	commentDefaults, err := MigrateCommentDefaults(db)
	if err != nil {
		log.Panicf("comment defaults migration failed: %v", err)
	}
	if commentDefaults > 0 {
		log.Printf("comment defaults migration complete: patched=%d", commentDefaults)
	}
	interactionVersions, err := MigrateArticleInteractions(db)
	if err != nil {
		log.Panicf("article interaction migration failed: %v", err)
	}
	if interactionVersions > 0 {
		log.Printf("article interaction migration complete: patched=%d", interactionVersions)
	}
	if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
		Name:   "interaction-user-target-version-type",
		Type:   "btree",
		Fields: []string{"userId", "targetKind", "targetId", "targetCreatedOn", "type"},
		Sparse: true,
		Unique: true,
	}); err != nil {
		log.Panicln("initialize interactions unique index:", err.Error())
	}
	if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
		Name:   "interactions-by-target-version",
		Type:   "btree",
		Fields: []string{"targetKind", "targetId", "targetCreatedOn", "id"},
		Sparse: true,
		Unique: true,
	}); err != nil {
		log.Panicln("initialize interactions target index:", err.Error())
	}
	if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
		Name:   "interactions-by-user-type-kind",
		Type:   "btree",
		Fields: []string{"userId", "type", "targetKind", "id"},
		Sparse: true,
		Unique: true,
	}); err != nil {
		log.Panicln("initialize interactions user index:", err.Error())
	}
	if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
		Name: "interactions-by-target-kind", Type: "btree", Fields: []string{"targetKind", "targetId", "id"}, Sparse: true, Unique: true,
	}); err != nil {
		log.Panicln("initialize interactions kind index:", err.Error())
	}
	if err := db.EnsureIndex(commentsCollection, &inceptiondb.IndexOptions{
		Name: "comments-by-article-version-parent-created", Type: "btree",
		Fields: []string{"articleId", "articleCreatedOn", "parentId", "createdOn", "id"}, Unique: true,
	}); err != nil {
		log.Panicln("initialize comments thread index:", err.Error())
	}
	if err := db.EnsureIndex(commentsCollection, &inceptiondb.IndexOptions{
		Name: "comments-by-user-created", Type: "btree", Fields: []string{"userId", "createdOn", "id"}, Unique: true,
	}); err != nil {
		log.Panicln("initialize comments user index:", err.Error())
	}

	b := box.NewBox()

	b.WithInterceptors(AccessLog)

	et, err := templates.NewEmbeddedTemplates()
	if err != nil {
		panic(err)
	}
	b.WithInterceptors(templates.Inject(et))

	b.WithInterceptors(InjectInceptionClient(db))
	b.WithInterceptors(glueauth.Auth)
	b.WithInterceptors(ResolveLocale)
	b.WithInterceptors(PrettyError)
	b.WithInterceptors(InjectContentModerator(moderator))

	// Public endpoints
	b.Handle("GET", "/", RenderHome)
	b.Handle("GET", "/blogs", RenderBlogs)
	b.Handle("GET", "/blogs/", RedirectBlogsSlash)
	b.Handle("GET", "/blog/{blogSlug}", RenderBlog)
	b.Handle("GET", "/blog/{blogSlug}/article/{articleUrl}", RenderBlogArticle)
	b.Handle("GET", "/blog/{blogSlug}/article/{articleUrl}/{language}", RenderTranslatedBlogArticle)
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
	v1.Handle("GET", "/search/articles", SearchPublicArticles)
	v1.Handle("GET", "/articles/{articleId}/comments", ListComments)
	v1.Handle("POST", "/articles/{articleId}/comments", CreateComment)
	v1.Handle("GET", "/comments/location/{fragment}", GetCommentLocation)
	v1.Handle("GET", "/articles/{articleId}/feedback", GetArticleFeedback).WithInterceptors(glueauth.Auth)
	v1.Handle("GET", "/blogs/{blogId}/feedback", GetBlogFeedback).WithInterceptors(glueauth.Auth)
	v1.Handle("GET", "/comments/{commentId}/feedback", GetCommentFeedback).WithInterceptors(glueauth.Auth)

	privateEndpoint := func(method, path string, handler any) *box.A {
		return v1.Handle(method, path, handler).WithInterceptors(glueauth.Require)
	}
	privateEndpoint("GET", "/capabilities", GetCapabilities)
	privateEndpoint("GET", "/preferences", GetUserPreferences)
	privateEndpoint("PUT", "/preferences", PutUserPreferences)
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
	privateEndpoint("PUT", "/blogs/{blogId}/like", LikeBlog)
	privateEndpoint("DELETE", "/blogs/{blogId}/like", UnlikeBlog)
	privateEndpoint("PUT", "/blogs/{blogId}/bookmark", BookmarkBlog)
	privateEndpoint("DELETE", "/blogs/{blogId}/bookmark", RemoveBlogBookmark)
	privateEndpoint("PUT", "/comments/{commentId}/like", LikeComment)
	privateEndpoint("DELETE", "/comments/{commentId}/like", UnlikeComment)
	privateEndpoint("PUT", "/comments/{commentId}/bookmark", BookmarkComment)
	privateEndpoint("DELETE", "/comments/{commentId}/bookmark", RemoveCommentBookmark)
	privateEndpoint("GET", "/bookmarks", ListBookmarks)
	privateEndpoint("GET", "/saved/{kind}", ListSaved)
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
