package api

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fulldump/apitest"
	"github.com/fulldump/box"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
	"gopress/inceptiondb"
)

type alwaysBanModerator struct{}

func (alwaysBanModerator) Evaluate(context.Context, string) (bool, error) { return true, nil }

func TestBannedArticleHidesBlogAndAllowsOwnerReview(t *testing.T) {
	db := newBlogTestDB(t)
	if err := db.CreateCollection("articles"); err != nil {
		t.Fatal(err)
	}
	createdOn := time.Now().Add(-time.Hour).UTC()
	if err := db.Insert("articles", &Article{
		Id: "banned-article", CreatedOn: createdOn, PublishOn: createdOn, Published: true, Banned: true,
		AuthorId: "owner", AuthorNick: "writer", ArticleUserFields: ArticleUserFields{
			Title: "Banned", Url: "banned", ContentHTML: template.HTML("<p>hidden</p>"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Insert("articles", &Article{
		Id: "clean-article", CreatedOn: createdOn.Add(time.Minute), PublishOn: createdOn.Add(time.Minute), Published: true,
		AuthorId: "owner", AuthorNick: "writer", ArticleUserFields: ArticleUserFields{
			Title: "Clean sibling", Url: "clean", Tags: []string{"clean-tag"}, ContentHTML: template.HTML("<p>also hidden</p>"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := box.Box2Http(NewApi("", "test", db, fs, nil))
	client := apitest.NewWithHandler(handler)
	requestAs := func(userId, method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication,
			`{"user":{"id":"`+userId+`","nick":"`+userId+`"}}`)
	}
	blogId := deterministicPersonalBlogId("owner")
	blog := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, blog); err != nil {
		t.Fatal(err)
	}
	if !blog.Banned || blog.BannedByArticleId != "banned-article" || blog.BannedOn == nil {
		t.Fatalf("blog was not banned: %+v", blog)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for _, path := range []string{
		"/blog/" + blog.Slug, "/blog/" + blog.Slug + "/article/banned", "/blog/" + blog.Slug + "/article/clean",
		"/user/writer", "/tag/clean-tag",
	} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("banned public route %s returned %d", path, response.StatusCode)
		}
	}
	response, err := http.Get(server.URL + "/blogs")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(body), "/blog/"+blog.Slug) {
		t.Fatalf("banned blog appears in directory: %s", body)
	}
	for _, path := range []string{"/", "/sitemap.xml"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if strings.Contains(string(body), "Clean sibling") || strings.Contains(string(body), "/blog/"+blog.Slug) {
			t.Fatalf("banned blog leaked from %s: %s", path, body)
		}
	}
	for _, path := range []string{"/v1/articles/clean-article/feedback", "/v1/articles/clean-article/comments"} {
		if publicAPI := client.Request(http.MethodGet, path).Do(); publicAPI.StatusCode != http.StatusNotFound {
			t.Fatalf("banned blog API %s returned %d %s", path, publicAPI.StatusCode, publicAPI.BodyString())
		}
	}

	owned := requestAs("owner", http.MethodGet, "/v1/blogs").Do()
	if owned.StatusCode != http.StatusOK || !strings.Contains(owned.BodyString(), `"banned":true`) || strings.Contains(owned.BodyString(), "review_requested_on") {
		t.Fatalf("owner cannot see banned blog: %d %s", owned.StatusCode, owned.BodyString())
	}
	if created := requestAs("owner", http.MethodPost, "/v1/blogs/"+blogId+"/articles").
		WithBodyJson(JSON{"title": "Blocked"}).Do(); created.StatusCode != http.StatusForbidden || errorCode(created) != "blog_banned" {
		t.Fatalf("owner created article in banned blog: %d %s", created.StatusCode, created.BodyString())
	}

	editor := &BlogMember{Id: deterministicBlogMemberId(blogId, "editor"), BlogId: blogId, UserId: "editor",
		UserNick: "editor", Role: BlogRoleEditor, CreatedOn: time.Now().UTC()}
	if err := db.Insert(blogMembersCollection, editor); err != nil {
		t.Fatal(err)
	}
	if listed := requestAs("editor", http.MethodGet, "/v1/blogs").Do(); listed.StatusCode != http.StatusOK || strings.Contains(listed.BodyString(), blogId) {
		t.Fatalf("editor can see banned blog: %d %s", listed.StatusCode, listed.BodyString())
	}
	if retrieved := requestAs("editor", http.MethodGet, "/v1/blogs/"+blogId).Do(); retrieved.StatusCode != http.StatusNotFound {
		t.Fatalf("editor opened banned blog: %d %s", retrieved.StatusCode, retrieved.BodyString())
	}

	review := requestAs("owner", http.MethodPost, "/v1/blogs/"+blogId+"/review-request").WithBodyJson(JSON{
		"message": "He revisado el contenido y solicito una revisión manual.",
	}).Do()
	if review.StatusCode != http.StatusCreated || !strings.Contains(review.BodyString(), "review_requested_on") {
		t.Fatalf("request review: %d %s", review.StatusCode, review.BodyString())
	}
	if duplicate := requestAs("owner", http.MethodPost, "/v1/blogs/"+blogId+"/review-request").WithBodyJson(JSON{
		"message": "Segunda solicitud de revisión.",
	}).Do(); duplicate.StatusCode != http.StatusConflict || errorCode(duplicate) != "review_already_requested" {
		t.Fatalf("duplicate review accepted: %d %s", duplicate.StatusCode, duplicate.BodyString())
	}
}

func TestPeriodicBlogBanReconciliationUsesLookback(t *testing.T) {
	db := newBlogTestDB(t)
	for _, collection := range []string{"articles", blogsCollection} {
		if err := db.CreateCollection(collection); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	for _, fixture := range []struct {
		id      string
		created time.Time
		updated time.Time
	}{
		{"recent", now.Add(-30 * 24 * time.Hour), now.Add(-time.Hour)},
		{"old", now.Add(-30 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour)},
	} {
		blog := &Blog{Id: "blog-" + fixture.id, Slug: "blog-" + fixture.id, Name: fixture.id,
			Language: BlogLanguageES, AccentColor: "#2563EB", OwnerId: "owner-" + fixture.id,
			CreatedOn: fixture.created, UpdatedOn: fixture.created}
		if err := db.Insert(blogsCollection, blog); err != nil {
			t.Fatal(err)
		}
		if err := db.Insert("articles", &Article{Id: "article-" + fixture.id, BlogId: blog.Id,
			CreatedOn: fixture.created, UpdatedOn: fixture.updated, Banned: true, AuthorId: blog.OwnerId}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := ReconcileBannedBlogs(db, now.Add(-blogBanLookback))
	if err != nil {
		t.Fatal(err)
	}
	if report.BlogsBanned != 1 || report.ArticlesMatched != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	for _, fixture := range []struct {
		id     string
		banned bool
	}{{"recent", true}, {"old", false}} {
		blog := &Blog{}
		if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": "blog-" + fixture.id}}, blog); err != nil {
			t.Fatal(err)
		}
		if blog.Banned != fixture.banned {
			t.Fatalf("blog %s banned=%v", fixture.id, blog.Banned)
		}
	}
}

func TestPublishingBannedContentImmediatelyBansBlog(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(NewApi("", "test", db, fs, alwaysBanModerator{}))
	request := func(method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication, `{"user":{"id":"owner","nick":"writer"}}`)
	}
	blogResponse := request(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{"slug": "moderated", "name": "Moderated"}).Do()
	if blogResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create blog: %d %s", blogResponse.StatusCode, blogResponse.BodyString())
	}
	blogId := blogResponse.BodyJsonMap()["id"].(string)
	articleResponse := request(http.MethodPost, "/v1/blogs/"+blogId+"/articles").WithBodyJson(JSON{"title": "Forbidden content"}).Do()
	if articleResponse.StatusCode != http.StatusOK {
		t.Fatalf("create article: %d %s", articleResponse.StatusCode, articleResponse.BodyString())
	}
	article := articleResponse.BodyJsonMap()
	articleRoot := "/v1/blogs/" + blogId + "/articles/" + article["id"].(string)
	lock := request(http.MethodPost, articleRoot+"/edit-lock/acquire").WithBodyJson(JSON{"clientId": "publish"}).Do()
	if lock.StatusCode != http.StatusOK {
		t.Fatalf("acquire lock: %d %s", lock.StatusCode, lock.BodyString())
	}
	published := request(http.MethodPost, articleRoot+"/publish").
		WithHeader(ArticleLockTokenHeader, lock.BodyJsonMap()["token"].(string)).
		WithBodyJson(JSON{"revision": article["revision"]}).Do()
	if published.StatusCode != http.StatusOK || !published.BodyJsonMap()["banned"].(bool) {
		t.Fatalf("publish banned article: %d %s", published.StatusCode, published.BodyString())
	}
	blog := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, blog); err != nil {
		t.Fatal(err)
	}
	if !blog.Banned || blog.BannedByArticleId != article["id"].(string) {
		t.Fatalf("blog was not immediately banned: %+v", blog)
	}
}
