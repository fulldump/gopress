package api

import (
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
		Id:        "published-article",
		Published: true,
		PublishOn: time.Now(),
		ArticleUserFields: ArticleUserFields{
			Title: "Published article",
		},
	}
	if err := db.Insert("articles", article); err != nil {
		t.Fatal(err)
	}

	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(NewApi("", "test-version", db, fs, nil))
	commentsPath := "/v1/articles/published-article/comments"

	t.Run("empty comment list", func(t *testing.T) {
		response := client.Request(http.MethodGet, commentsPath).Do()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", response.StatusCode)
		}
		comments := response.BodyJson().([]any)
		if len(comments) != 0 {
			t.Fatalf("expected no comments, got %d", len(comments))
		}
	})

	markdown := "**Buen aporte**\n\n```go\nfmt.Println(\"hola\")\n```\n<script>alert(1)</script>"
	rootResponse := client.Request(http.MethodPost, commentsPath).WithBodyJson(JSON{
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
		comments := response.BodyJson().([]any)
		if len(comments) != 1 {
			t.Fatalf("expected one root comment, got %d", len(comments))
		}
		rootComment := comments[0].(map[string]any)
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
}
