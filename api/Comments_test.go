package api

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fulldump/apitest"
	"github.com/fulldump/box"
	inceptionapi "github.com/fulldump/inceptiondb/api"
	"github.com/fulldump/inceptiondb/database"
	"github.com/fulldump/inceptiondb/service"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
	"gopress/inceptiondb"
)

func TestComments(t *testing.T) {
	store := database.NewDatabase(&database.Config{Dir: t.TempDir()})
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Stop(); err != nil {
			t.Error(err)
		}
	})

	databaseAPI := inceptionapi.Build(service.NewService(store), "", "embedded")
	databaseAPI.WithInterceptors(
		inceptionapi.AccessLog(log.New(os.Stdout, "ACCESS: ", log.Lshortfile)),
		inceptionapi.RecoverFromPanic,
		inceptionapi.PrettyErrorInterceptor,
		inceptionapi.InterceptorUnavailable(store),
	)
	databaseServer := httptest.NewServer(box.Box2Http(databaseAPI))
	t.Cleanup(databaseServer.Close)

	db := inceptiondb.NewClient(inceptiondb.Config{Base: databaseServer.URL + "/v1"})
	article := &Article{
		Id:         "published-article",
		CreatedOn:  time.Now().Add(-time.Hour).UTC(),
		Published:  true,
		PublishOn:  time.Now(),
		AuthorId:   "comment-author",
		AuthorNick: "writer",
		ArticleUserFields: ArticleUserFields{
			Title: "Published article",
		},
	}
	if err := db.Insert("articles", article); err != nil {
		t.Fatal(err)
	}
	if err := db.Insert(commentsCollection, JSON{
		"id": "comment_legacy-anonymous", "articleId": article.Id, "articleCreatedOn": article.CreatedOn.Add(-time.Minute),
		"name": "Legacy", "text": "Anonymous", "createdOn": time.Now().Add(-time.Hour).UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(NewApi("", "test-version", db, fs, nil))
	legacy := &Comment{}
	if err := db.FindOne(commentsCollection, inceptiondb.FindQuery{Filter: JSON{"id": "comment_legacy-anonymous"}}, legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.UserId != "" || legacy.ParentId != "" {
		t.Fatalf("legacy anonymous comment gained ownership or parent: %#v", legacy)
	}
	if count, err := MigrateCommentDefaults(db); err != nil || count != 0 {
		t.Fatalf("comment defaults migration must be idempotent, count=%d err=%v", count, err)
	}
	commentsPath := "/v1/articles/published-article/comments"
	userA := `{"user":{"id":"comment-user-a","nick":"Ana"}}`
	userB := `{"user":{"id":"comment-user-b","nick":"Bea"}}`
	authRequest := func(method, path, auth string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication, auth)
	}

	t.Run("empty comment list", func(t *testing.T) {
		response := client.Request(http.MethodGet, commentsPath).Do()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", response.StatusCode)
		}
		comments := response.BodyJsonMap()["items"].([]any)
		if len(comments) != 0 {
			t.Fatalf("expected no comments, got %d", len(comments))
		}
	})

	markdown := "**Buen aporte**\n\n```go\nfmt.Println(\"hola\")\n```\n<script>alert(1)</script>"
	rootResponse := authRequest(http.MethodPost, commentsPath, userA).WithBodyJson(JSON{
		"name":  "Ana",
		"email": "ana@example.com",
		"text":  markdown,
	}).Do()
	if rootResponse.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rootResponse.StatusCode)
	}
	root := rootResponse.BodyJsonMap()
	if root["text"] != markdown {
		t.Fatalf("expected raw markdown %q, got %q", markdown, root["text"])
	}
	if _, exposed := root["email"]; exposed {
		t.Fatal("public comment response must not expose email")
	}
	if _, exposed := root["userId"]; exposed {
		t.Fatal("public comment response must not expose userId")
	}
	if root["mine"] != true {
		t.Fatalf("authenticated author should see mine=true: %#v", root)
	}
	if !strings.HasSuffix(root["url"].(string), "#comment-"+strings.TrimPrefix(root["id"].(string), "comment_")) {
		t.Fatalf("unexpected canonical comment URL: %q", root["url"])
	}
	rootId := root["id"].(string)
	storedRoot := &Comment{}
	if err := db.FindOne(commentsCollection, inceptiondb.FindQuery{
		Filter: JSON{"id": rootId},
	}, storedRoot); err != nil {
		t.Fatal(err)
	}
	if storedRoot.Email != "ana@example.com" {
		t.Fatalf("expected email to be persisted, got %q", storedRoot.Email)
	}
	if storedRoot.UserId != "comment-user-a" {
		t.Fatalf("expected authenticated user id to be persisted, got %q", storedRoot.UserId)
	}

	replyResponse := client.Request(http.MethodPost, commentsPath).WithBodyJson(JSON{
		"parentId": rootId,
		"name":     "Luis",
		"text":     "Gracias, `Ana`.",
	}).Do()
	if replyResponse.StatusCode != http.StatusCreated {
		t.Fatalf("expected reply status 201, got %d", replyResponse.StatusCode)
	}
	reply := replyResponse.BodyJsonMap()
	replyId := reply["id"].(string)

	t.Run("only one reply level", func(t *testing.T) {
		response := client.Request(http.MethodPost, commentsPath).WithBodyJson(JSON{
			"parentId": replyId,
			"name":     "Eva",
			"text":     "Nested reply",
		}).Do()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", response.StatusCode)
		}
	})

	t.Run("nested list", func(t *testing.T) {
		response := client.Request(http.MethodGet, commentsPath).Do()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", response.StatusCode)
		}
		comments := response.BodyJsonMap()["items"].([]any)
		if len(comments) != 1 {
			t.Fatalf("expected one root comment, got %d", len(comments))
		}
		rootComment := comments[0].(map[string]any)
		if rootComment["mine"] != false {
			t.Fatalf("anonymous read must not reveal ownership: %#v", rootComment)
		}
		if _, exposed := rootComment["userId"]; exposed {
			t.Fatal("list response exposed userId")
		}
		replies := rootComment["replies"].([]any)
		if len(replies) != 1 {
			t.Fatalf("expected one reply, got %d", len(replies))
		}
		if replies[0].(map[string]any)["parentId"] != rootId {
			t.Fatal("reply was not attached to its parent")
		}
	})

	t.Run("invalid email", func(t *testing.T) {
		response := client.Request(http.MethodPost, commentsPath).WithBodyJson(JSON{
			"name":  "Ana",
			"email": "not-an-email",
			"text":  "Hello",
		}).Do()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", response.StatusCode)
		}
	})

	t.Run("unpublished or missing article", func(t *testing.T) {
		response := client.Request(http.MethodGet, "/v1/articles/missing/comments").Do()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", response.StatusCode)
		}
	})

	t.Run("blog and comment feedback is public and mutations are private", func(t *testing.T) {
		if err := db.EnsureIndex(articleInteractionsCollection, &inceptiondb.IndexOptions{
			Name: "interaction-user-article-version-type", Type: "btree",
			Fields: []string{"userId", "articleId", "articleCreatedOn", "type"}, Unique: true,
		}); err != nil {
			t.Fatal(err)
		}
		blogId := deterministicPersonalBlogId("comment-author")
		blogFeedback := "/v1/blogs/" + blogId + "/feedback"
		blogLike := "/v1/blogs/" + blogId + "/like"
		if response := client.Request(http.MethodPut, blogLike).Do(); response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous blog mutation status=%d", response.StatusCode)
		}
		for i := 0; i < 2; i++ {
			feedback := authRequest(http.MethodPut, blogLike, userA).Do().BodyJsonMap()
			if fmt.Sprint(feedback["likes"]) != "1" || feedback["liked"] != true {
				t.Fatalf("blog like is not idempotent: %#v", feedback)
			}
		}
		anonymous := client.Request(http.MethodGet, blogFeedback).Do().BodyJsonMap()
		if fmt.Sprint(anonymous["likes"]) != "1" || anonymous["liked"] != false {
			t.Fatalf("unexpected anonymous blog feedback: %#v", anonymous)
		}

		commentFeedback := "/v1/comments/" + rootId + "/feedback"
		commentLike := "/v1/comments/" + rootId + "/like"
		commentBookmark := "/v1/comments/" + rootId + "/bookmark"
		if response := client.Request(http.MethodPut, commentLike).Do(); response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous comment mutation status=%d", response.StatusCode)
		}
		for i := 0; i < 2; i++ {
			feedback := authRequest(http.MethodPut, commentLike, userA).Do().BodyJsonMap()
			if fmt.Sprint(feedback["likes"]) != "1" {
				t.Fatalf("comment like is not idempotent: %#v", feedback)
			}
		}
		other := authRequest(http.MethodGet, commentFeedback, userB).Do().BodyJsonMap()
		if fmt.Sprint(other["likes"]) != "1" || other["liked"] != false {
			t.Fatalf("comment feedback leaked another user's state: %#v", other)
		}
		if response := authRequest(http.MethodPut, commentBookmark, userA).Do(); response.StatusCode != http.StatusOK {
			t.Fatalf("bookmark comment status=%d", response.StatusCode)
		}
		if response := authRequest(http.MethodPut, "/v1/blogs/"+blogId+"/bookmark", userA).Do(); response.StatusCode != http.StatusOK {
			t.Fatalf("bookmark blog status=%d", response.StatusCode)
		}
	})

	rootIds := []string{rootId}
	for i := 1; i < 25; i++ {
		response := authRequest(http.MethodPost, commentsPath, userA).WithBodyJson(JSON{
			"name": "Ana", "text": fmt.Sprintf("Root %02d", i),
		}).Do()
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create pagination root %d status=%d", i, response.StatusCode)
		}
		rootIds = append(rootIds, response.BodyJsonMap()["id"].(string))
	}

	t.Run("root pagination keeps direct replies together", func(t *testing.T) {
		first := client.Request(http.MethodGet, commentsPath).Do().BodyJsonMap()
		firstItems := first["items"].([]any)
		if len(firstItems) != 20 || first["hasMore"] != true || fmt.Sprint(first["nextOffset"]) != "20" {
			t.Fatalf("unexpected first comments page: %#v", first)
		}
		if len(firstItems[0].(map[string]any)["replies"].([]any)) != 1 {
			t.Fatal("first root was split from its direct reply")
		}
		rootFeedback := firstItems[0].(map[string]any)["feedback"].(map[string]any)
		if fmt.Sprint(rootFeedback["likes"]) != "1" || rootFeedback["liked"] != false || rootFeedback["bookmarked"] != false {
			t.Fatalf("comment list did not include private-safe feedback: %#v", rootFeedback)
		}
		second := client.Request(http.MethodGet, commentsPath+"?offset=20").Do().BodyJsonMap()
		if len(second["items"].([]any)) != 5 || second["hasMore"] != false || fmt.Sprint(second["nextOffset"]) != "25" {
			t.Fatalf("unexpected second comments page: %#v", second)
		}
		location := client.Request(http.MethodGet, "/v1/comments/location/"+strings.TrimPrefix(rootIds[22], "comment_")).Do()
		if location.StatusCode != http.StatusOK || fmt.Sprint(location.BodyJsonMap()["offset"]) != "20" {
			t.Fatalf("unexpected comment location: %d %s", location.StatusCode, location.BodyString())
		}
		replyLocation := client.Request(http.MethodGet, "/v1/comments/location/"+strings.TrimPrefix(replyId, "comment_")).Do()
		if replyLocation.StatusCode != http.StatusOK || fmt.Sprint(replyLocation.BodyJsonMap()["offset"]) != "0" {
			t.Fatalf("unexpected reply location: %d %s", replyLocation.StatusCode, replyLocation.BodyString())
		}
	})

	for _, id := range rootIds {
		response := authRequest(http.MethodPut, "/v1/comments/"+id+"/bookmark", userA).Do()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("bookmark %q status=%d", id, response.StatusCode)
		}
	}

	t.Run("saved sections are private, paginated, contextual, and ownership excludes anonymous replies", func(t *testing.T) {
		comments := authRequest(http.MethodGet, "/v1/saved/comments", userA).Do().BodyJsonMap()
		if len(comments["items"].([]any)) != 20 || comments["hasMore"] != true {
			t.Fatalf("unexpected saved comments page: %#v", comments)
		}
		item := comments["items"].([]any)[0].(map[string]any)
		if item["article"] == nil || item["blog"] == nil || !strings.Contains(item["url"].(string), "#comment-") {
			t.Fatalf("saved comment lacks context or canonical URL: %#v", item)
		}
		second := authRequest(http.MethodGet, "/v1/saved/comments?offset=20", userA).Do().BodyJsonMap()
		if len(second["items"].([]any)) != 5 || second["hasMore"] != false {
			t.Fatalf("unexpected second saved page: %#v", second)
		}
		own := authRequest(http.MethodGet, "/v1/saved/own-comments", userA).Do().BodyJsonMap()
		if len(own["items"].([]any)) != 20 || own["hasMore"] != true {
			t.Fatalf("unexpected own comments page: %#v", own)
		}
		ownSecond := authRequest(http.MethodGet, "/v1/saved/own-comments?offset=20", userA).Do().BodyJsonMap()
		if len(ownSecond["items"].([]any)) != 5 {
			t.Fatalf("legacy anonymous reply became owned: %#v", ownSecond)
		}
		other := authRequest(http.MethodGet, "/v1/saved/comments", userB).Do().BodyJsonMap()
		if len(other["items"].([]any)) != 0 {
			t.Fatalf("saved comments leaked across users: %#v", other)
		}
		blogs := authRequest(http.MethodGet, "/v1/saved/blogs", userA).Do().BodyJsonMap()
		if len(blogs["items"].([]any)) != 1 {
			t.Fatalf("unexpected saved blogs: %#v", blogs)
		}
	})

	t.Run("stale comments and banned blogs reject interactions and disappear from saved", func(t *testing.T) {
		stale := &Comment{Id: "comment_stale", ArticleId: article.Id, ArticleCreatedOn: article.CreatedOn.Add(-time.Minute),
			Name: "Old", Text: "Old version", CreatedOn: time.Now().UTC()}
		if err := db.Insert(commentsCollection, stale); err != nil {
			t.Fatal(err)
		}
		if response := authRequest(http.MethodPut, "/v1/comments/comment_stale/like", userA).Do(); response.StatusCode != http.StatusNotFound {
			t.Fatalf("stale comment interaction status=%d", response.StatusCode)
		}
		blogId := deterministicPersonalBlogId("comment-author")
		patched := &Blog{}
		if matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{"id": blogId}, Patch: JSON{"banned": true}}, patched); err != nil || !matched {
			t.Fatalf("ban blog matched=%t err=%v", matched, err)
		}
		if response := authRequest(http.MethodPut, "/v1/blogs/"+blogId+"/like", userA).Do(); response.StatusCode != http.StatusNotFound {
			t.Fatalf("banned blog interaction status=%d", response.StatusCode)
		}
		if response := authRequest(http.MethodPut, "/v1/comments/"+rootId+"/like", userA).Do(); response.StatusCode != http.StatusNotFound {
			t.Fatalf("comment in banned blog interaction status=%d", response.StatusCode)
		}
		if items := authRequest(http.MethodGet, "/v1/saved/comments", userA).Do().BodyJsonMap()["items"].([]any); len(items) != 0 {
			t.Fatalf("banned comments remained saved: %#v", items)
		}
		if items := authRequest(http.MethodGet, "/v1/saved/blogs", userA).Do().BodyJsonMap()["items"].([]any); len(items) != 0 {
			t.Fatalf("banned blog remained saved: %#v", items)
		}
		if items := authRequest(http.MethodGet, "/v1/saved/own-comments", userA).Do().BodyJsonMap()["items"].([]any); len(items) != 0 {
			t.Fatalf("comments in a banned blog remained in own comments: %#v", items)
		}
	})
}
