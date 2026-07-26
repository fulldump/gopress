package api

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fulldump/box"

	"gopress/filestorage/localfilestore"
)

func TestPublicBlogRoutesAndLegacyRedirects(t *testing.T) {
	db := newBlogTestDB(t)
	if err := db.CreateCollection("articles"); err != nil {
		t.Fatal(err)
	}
	createdOn := time.Now().Add(-time.Hour).UTC()
	article := &Article{
		Id: "legacy-cpp", CreatedOn: createdOn, PublishOn: createdOn, Published: true,
		AuthorId: "writer-id", AuthorNick: "writer", ArticleUserFields: ArticleUserFields{
			Title: "C++ without surprises", Url: "c++", ContentHTML: template.HTML("<p>Useful article.</p>"),
		},
	}
	if err := db.Insert("articles", article); err != nil {
		t.Fatal(err)
	}
	if err := db.Insert("articles", &Article{
		Id: "draft-only", CreatedOn: createdOn, Published: false,
		AuthorId: "draft-writer-id", AuthorNick: "draftwriter", ArticleUserFields: ArticleUserFields{
			Title: "Private draft", Url: "private-draft",
		},
	}); err != nil {
		t.Fatal(err)
	}
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(box.Box2Http(NewApi("", "test", db, fs, nil)))
	t.Cleanup(server.Close)
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}

	get := func(path string) (*http.Response, string) {
		t.Helper()
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return response, string(body)
	}

	if response, body := get("/blogs"); response.StatusCode != http.StatusOK || !strings.Contains(body, "writer") || strings.Contains(body, "draftwriter") || !strings.Contains(body, `id="blog-search"`) {
		t.Fatalf("blog directory: status=%d body=%s", response.StatusCode, body)
	}
	if response, body := get("/v1/search/articles?q=surprises"); response.StatusCode != http.StatusOK {
		t.Fatalf("article search: status=%d body=%s", response.StatusCode, body)
	} else {
		payload := struct {
			Items    []PublicArticleSearchResult `json:"items"`
			Articles []PublicArticleSearchResult `json:"articles"`
		}{}
		if err := json.Unmarshal([]byte(body), &payload); err != nil || len(payload.Items) != 1 || payload.Items[0].Title != "C++ without surprises" || payload.Items[0].PublicUrl != "/blog/writer/article/c++" {
			t.Fatalf("unexpected article search payload: err=%v body=%s", err, body)
		}
	}
	if response, body := get("/v1/search/articles?q=private"); response.StatusCode != http.StatusOK || strings.Contains(body, "Private draft") {
		t.Fatalf("draft leaked through article search: status=%d body=%s", response.StatusCode, body)
	}
	if response, _ := get("/blogs/"); response.StatusCode != http.StatusPermanentRedirect || response.Header.Get("Location") != "/blogs" {
		t.Fatalf("blogs slash redirect: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
	if response, body := get("/blog/writer"); response.StatusCode != http.StatusOK || !strings.Contains(body, "Useful article") {
		t.Fatalf("blog page: status=%d body=%s", response.StatusCode, body)
	}
	if response, body := get("/blog/writer/article/c++"); response.StatusCode != http.StatusOK || !strings.Contains(body, "Useful article") || !strings.Contains(body, `rel="canonical"`) {
		t.Fatalf("canonical article: status=%d body=%s", response.StatusCode, body)
	}
	if response, _ := get("/user/writer/article/c++"); response.StatusCode != http.StatusPermanentRedirect || response.Header.Get("Location") != "/blog/writer/article/c++" {
		t.Fatalf("legacy article redirect: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
	if response, body := get("/sitemap.xml"); response.StatusCode != http.StatusOK || !strings.Contains(body, "/blog/writer/article/c++") || !strings.Contains(body, "sitemaps.org") {
		t.Fatalf("sitemap: status=%d body=%s", response.StatusCode, body)
	}
}
