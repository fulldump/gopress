package api

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestArticleFeedback(t *testing.T) {
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
	originalCreatedOn := time.Now().Add(-time.Hour).UTC()
	for _, article := range []*Article{
		{
			Id:         "published-article",
			CreatedOn:  originalCreatedOn,
			Published:  true,
			PublishOn:  time.Now(),
			AuthorId:   "user-a",
			AuthorNick: "writer",
			ArticleUserFields: ArticleUserFields{
				Title: "Published article",
				Url:   "published-article",
			},
		},
		{
			Id:         "draft-article",
			CreatedOn:  originalCreatedOn.Add(time.Minute),
			Published:  false,
			AuthorId:   "user-a",
			AuthorNick: "writer",
			ArticleUserFields: ArticleUserFields{
				Title: "Draft article",
			},
		},
	} {
		if err := db.Insert("articles", article); err != nil {
			t.Fatal(err)
		}
	}
	legacyInteraction := &ArticleInteraction{
		Id: "interaction-legacy", ArticleId: "published-article", ArticleCreatedOn: originalCreatedOn,
		UserId: "migration-user", Type: interactionBookmark, CreatedOn: time.Now().UTC(),
	}
	if err := db.Insert(articleInteractionsCollection, legacyInteraction); err != nil {
		t.Fatal(err)
	}

	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(NewApi("", "test-version", db, fs, nil))
	migrated := &ArticleInteraction{}
	if err := db.FindOne(articleInteractionsCollection, inceptiondb.FindQuery{Filter: JSON{"id": legacyInteraction.Id}}, migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.TargetKind != targetArticle || migrated.TargetId != "published-article" || !migrated.TargetCreatedOn.Equal(originalCreatedOn) {
		t.Fatalf("legacy interaction was not migrated: %#v", migrated)
	}
	if count, err := MigrateArticleInteractions(db); err != nil || count != 0 {
		t.Fatalf("interaction migration must be idempotent, count=%d err=%v", count, err)
	}
	feedbackPath := "/v1/articles/published-article/feedback"
	likePath := "/v1/articles/published-article/like"
	bookmarkPath := "/v1/articles/published-article/bookmark"
	userA := `{"user":{"id":"user-a","nick":"ana"}}`
	userB := `{"user":{"id":"user-b","nick":"bea"}}`
	authRequest := func(method, path, auth string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication, auth)
	}

	t.Run("anonymous feedback and protected actions", func(t *testing.T) {
		response := client.Request(http.MethodGet, feedbackPath).Do()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", response.StatusCode)
		}
		feedback := response.BodyJsonMap()
		if fmt.Sprint(feedback["likes"]) != "0" || feedback["liked"] != false || feedback["bookmarked"] != false {
			t.Fatalf("unexpected anonymous feedback: %#v", feedback)
		}

		response = client.Request(http.MethodPut, likePath).Do()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", response.StatusCode)
		}

		response = authRequest(http.MethodPut, likePath, `{"user":{}}`).Do()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected malformed auth status 401, got %d", response.StatusCode)
		}
	})

	t.Run("likes are idempotent and isolated", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			response := authRequest(http.MethodPut, likePath, userA).Do()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", response.StatusCode)
			}
			feedback := response.BodyJsonMap()
			if fmt.Sprint(feedback["likes"]) != "1" || feedback["liked"] != true {
				t.Fatalf("unexpected feedback after like: %#v", feedback)
			}
		}

		feedback := authRequest(http.MethodGet, feedbackPath, userB).Do().BodyJsonMap()
		if fmt.Sprint(feedback["likes"]) != "1" || feedback["liked"] != false {
			t.Fatalf("another user must see the count but not own the like: %#v", feedback)
		}

		feedback = authRequest(http.MethodPut, likePath, userB).Do().BodyJsonMap()
		if fmt.Sprint(feedback["likes"]) != "2" || feedback["liked"] != true {
			t.Fatalf("unexpected second user feedback: %#v", feedback)
		}

		feedback = authRequest(http.MethodDelete, likePath, userA).Do().BodyJsonMap()
		if fmt.Sprint(feedback["likes"]) != "1" || feedback["liked"] != false {
			t.Fatalf("unexpected feedback after unlike: %#v", feedback)
		}
	})

	t.Run("bookmarks list and removal", func(t *testing.T) {
		feedback := authRequest(http.MethodPut, bookmarkPath, userA).Do().BodyJsonMap()
		if feedback["bookmarked"] != true {
			t.Fatalf("expected article to be bookmarked: %#v", feedback)
		}

		response := authRequest(http.MethodGet, "/v1/bookmarks", userA).Do()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", response.StatusCode)
		}
		bookmarks := response.BodyJson().([]any)
		if len(bookmarks) != 1 || bookmarks[0].(map[string]any)["id"] != "published-article" {
			t.Fatalf("unexpected bookmarks: %#v", bookmarks)
		}
		saved := authRequest(http.MethodGet, "/v1/saved/articles", userA).Do().BodyJsonMap()
		if len(saved["items"].([]any)) != 1 || saved["hasMore"] != false || fmt.Sprint(saved["nextOffset"]) != "1" {
			t.Fatalf("unexpected saved articles envelope: %#v", saved)
		}

		otherBookmarks := authRequest(http.MethodGet, "/v1/bookmarks", userB).Do().BodyJson().([]any)
		if len(otherBookmarks) != 0 {
			t.Fatalf("bookmarks must be private per user: %#v", otherBookmarks)
		}
		if otherSaved := authRequest(http.MethodGet, "/v1/saved/articles", userB).Do().BodyJsonMap()["items"].([]any); len(otherSaved) != 0 {
			t.Fatalf("saved articles must be private per user: %#v", otherSaved)
		}

		feedback = authRequest(http.MethodDelete, bookmarkPath, userA).Do().BodyJsonMap()
		if feedback["bookmarked"] != false {
			t.Fatalf("expected bookmark removal: %#v", feedback)
		}
		bookmarks = authRequest(http.MethodGet, "/v1/bookmarks", userA).Do().BodyJson().([]any)
		if len(bookmarks) != 0 {
			t.Fatalf("expected empty bookmarks after removal: %#v", bookmarks)
		}
	})

	t.Run("drafts cannot receive interactions", func(t *testing.T) {
		response := authRequest(http.MethodPut, "/v1/articles/draft-article/like", userA).Do()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", response.StatusCode)
		}
	})

	t.Run("deleting an article removes its feedback", func(t *testing.T) {
		exactComment := &Comment{Id: "comment_exact-cleanup", ArticleId: "published-article", ArticleCreatedOn: originalCreatedOn,
			Name: "Exact", Text: "Delete me", CreatedOn: time.Now().UTC()}
		staleComment := &Comment{Id: "comment_stale-cleanup", ArticleId: "published-article", ArticleCreatedOn: originalCreatedOn.Add(-time.Minute),
			Name: "Stale", Text: "Keep me", CreatedOn: time.Now().Add(time.Millisecond).UTC()}
		for _, comment := range []*Comment{exactComment, staleComment} {
			if err := db.Insert(commentsCollection, comment); err != nil {
				t.Fatal(err)
			}
			if err := db.Insert(articleInteractionsCollection, &ArticleInteraction{
				Id: "interaction-" + comment.Id, TargetKind: targetComment, TargetId: comment.Id,
				TargetCreatedOn: comment.CreatedOn, UserId: userA, Type: interactionLike, CreatedOn: time.Now().UTC(),
			}); err != nil {
				t.Fatal(err)
			}
		}
		response := authRequest(http.MethodDelete, "/v1/articles/published-article", userB).Do()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("expected non-owner status 404, got %d", response.StatusCode)
		}
		feedback := client.Request(http.MethodGet, feedbackPath).Do().BodyJsonMap()
		if fmt.Sprint(feedback["likes"]) != "1" {
			t.Fatalf("non-owner deletion changed feedback: %#v", feedback)
		}

		lockResponse := authRequest(http.MethodPost, "/v1/articles/published-article/edit-lock/acquire", userA).
			WithBodyJson(JSON{"clientId": "feedback-test"}).Do()
		if lockResponse.StatusCode != http.StatusOK {
			t.Fatalf("expected lock status 200, got %d: %s", lockResponse.StatusCode, lockResponse.BodyString())
		}
		lockToken := lockResponse.BodyJsonMap()["token"].(string)
		response = authRequest(http.MethodDelete, "/v1/articles/published-article", userA).
			WithHeader(ArticleLockTokenHeader, lockToken).
			WithBodyJson(JSON{"revision": 1}).Do()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", response.StatusCode)
		}

		interaction := &ArticleInteraction{}
		err := db.FindOne(articleInteractionsCollection, inceptiondb.FindQuery{
			Filter: JSON{"articleId": "published-article"},
		}, interaction)
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected deleted feedback, got %#v and error %v", interaction, err)
		}
		if err := db.FindOne(commentsCollection, inceptiondb.FindQuery{Filter: JSON{"id": exactComment.Id}}, &Comment{}); !errors.Is(err, io.EOF) {
			t.Fatalf("exact-version comment was not deleted: %v", err)
		}
		if err := db.FindOne(articleInteractionsCollection, inceptiondb.FindQuery{Filter: JSON{"targetId": exactComment.Id}}, &ArticleInteraction{}); !errors.Is(err, io.EOF) {
			t.Fatalf("exact-version comment interaction was not deleted: %v", err)
		}
		if err := db.FindOne(commentsCollection, inceptiondb.FindQuery{Filter: JSON{"id": staleComment.Id}}, &Comment{}); err != nil {
			t.Fatalf("stale-version comment should remain: %v", err)
		}
		if err := db.FindOne(articleInteractionsCollection, inceptiondb.FindQuery{Filter: JSON{"targetId": staleComment.Id}}, &ArticleInteraction{}); err != nil {
			t.Fatalf("stale-version comment interaction should remain: %v", err)
		}
		staleInteraction := &ArticleInteraction{
			Id:               "interaction-stale",
			ArticleId:        "published-article",
			ArticleCreatedOn: originalCreatedOn,
			UserId:           "user-b",
			Type:             interactionLike,
			CreatedOn:        time.Now().UTC(),
		}
		if err := db.Insert(articleInteractionsCollection, staleInteraction); err != nil {
			t.Fatal(err)
		}

		replacement := &Article{
			Id:         "published-article",
			BlogId:     deterministicPersonalBlogId("user-a"),
			BlogSlug:   "writer",
			CreatedOn:  time.Now().UTC(),
			Published:  true,
			PublishOn:  time.Now(),
			AuthorId:   "user-a",
			AuthorNick: "writer",
			ArticleUserFields: ArticleUserFields{
				Title: "Replacement article",
				Url:   "replacement-article",
			},
		}
		if err := db.Insert("articles", replacement); err != nil {
			t.Fatal(err)
		}
		feedback = client.Request(http.MethodGet, feedbackPath).Do().BodyJsonMap()
		if fmt.Sprint(feedback["likes"]) != "0" {
			t.Fatalf("replacement inherited feedback: %#v", feedback)
		}
	})
}
