package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fulldump/box"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
	"gopress/inceptiondb"
)

func TestIncrementalPagination(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := box.Box2Http(NewApi("", "test", db, fs, nil))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	stamp := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	blog := &Blog{
		Id: "pagination-blog", Slug: "pagination-blog", Name: "Pagination", Language: BlogLanguageEN,
		AccentColor: "#2563EB", OwnerId: "owner", OwnerNick: "owner", CreatedOn: stamp, UpdatedOn: stamp,
		Members: []BlogMember{{Id: deterministicBlogMemberId("pagination-blog", "owner"), BlogId: "pagination-blog",
			UserId: "owner", UserNick: "owner", Role: BlogRoleOwner, CreatedOn: stamp}}, MembersRevision: 1,
	}
	if err := db.Insert(blogsCollection, blog); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 25; index++ {
		id := fmt.Sprintf("article-%02d", index)
		if err := db.Insert("articles", &Article{
			Id: id, BlogId: blog.Id, BlogSlug: blog.Slug, CreatedOn: stamp, UpdatedOn: stamp, PublishOn: stamp,
			Published: true, AuthorId: "owner", AuthorNick: "owner",
			ArticleUserFields: ArticleUserFields{Title: "Needle " + id, Url: id, Tags: []string{"pagination"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// These sort ahead of the published rows but must not consume public/search page slots.
	for index := 0; index < 7; index++ {
		id := fmt.Sprintf("draft-%02d", index)
		if err := db.Insert("articles", &Article{
			Id: id, BlogId: blog.Id, BlogSlug: blog.Slug, CreatedOn: stamp.Add(time.Hour), UpdatedOn: stamp.Add(time.Hour),
			PublishOn: stamp.Add(time.Hour), Published: false, AuthorId: "owner", AuthorNick: "owner",
			ArticleUserFields: ArticleUserFields{Title: "Needle " + id, Url: id},
		}); err != nil {
			t.Fatal(err)
		}
	}

	get := func(path string, authenticated bool) (*http.Response, []byte) {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if authenticated {
			request.Header.Set(glueauth.XGlueAuthentication, `{"user":{"id":"owner","nick":"owner"}}`)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return response, body
	}

	itemPattern := regexp.MustCompile(`data-item-id="([^"]+)"`)
	firstResponse, firstHTML := get("/?lang=en", false)
	secondResponse, secondHTML := get("/?lang=en&offset=20", false)
	if firstResponse.StatusCode != http.StatusOK || secondResponse.StatusCode != http.StatusOK {
		t.Fatalf("public pages returned %d and %d", firstResponse.StatusCode, secondResponse.StatusCode)
	}
	firstIDs := paginationHTMLIDs(itemPattern, firstHTML)
	secondIDs := paginationHTMLIDs(itemPattern, secondHTML)
	if len(firstIDs) != 20 || len(secondIDs) != 5 {
		t.Fatalf("public page sizes = %d and %d, want 20 and 5", len(firstIDs), len(secondIDs))
	}
	assertNoDuplicateIDs(t, append(append([]string{}, firstIDs...), secondIDs...))
	if firstIDs[0] != "article-24" || firstIDs[19] != "article-05" || secondIDs[0] != "article-04" {
		t.Fatalf("public ordering is not deterministic: first=%v second=%v", firstIDs, secondIDs)
	}
	if !strings.Contains(string(firstHTML), `href="/?lang=en&amp;offset=20"`) || strings.Contains(string(secondHTML), "data-pagination-control") {
		t.Fatalf("public next-page control did not preserve lang or terminate correctly")
	}

	firstSearch := decodeSearchPage(t, getBody(t, get, "/v1/search/articles?q=needle", false))
	secondSearch := decodeSearchPage(t, getBody(t, get, "/v1/search/articles?q=needle&offset=20", false))
	if len(firstSearch.Items) != 20 || !firstSearch.HasMore || firstSearch.NextOffset != 20 || len(secondSearch.Items) != 5 || secondSearch.HasMore || secondSearch.NextOffset != 25 {
		t.Fatalf("unexpected search pages: first=%+v second=%+v", firstSearch, secondSearch)
	}
	searchIDs := make([]string, 0, 25)
	for _, item := range append(firstSearch.Items, secondSearch.Items...) {
		if strings.HasPrefix(item.Id, "draft-") {
			t.Fatalf("draft passed the search filter before pagination: %s", item.Id)
		}
		searchIDs = append(searchIDs, item.Id)
	}
	assertNoDuplicateIDs(t, searchIDs)
	if searchIDs[0] != "article-24" || searchIDs[24] != "article-00" {
		t.Fatalf("search tie ordering is not deterministic: %v", searchIDs)
	}

	publishedPage := ListPage[*ArticleShort]{}
	if err := json.Unmarshal(getBody(t, get, "/v1/blogs/"+blog.Id+"/articles?published=true", true), &publishedPage); err != nil {
		t.Fatal(err)
	}
	draftPage := ListPage[*ArticleShort]{}
	if err := json.Unmarshal(getBody(t, get, "/v1/blogs/"+blog.Id+"/articles?published=false", true), &draftPage); err != nil {
		t.Fatal(err)
	}
	if len(publishedPage.Items) != 20 || !publishedPage.HasMore || len(draftPage.Items) != 7 || draftPage.HasMore {
		t.Fatalf("publication filters were applied after pagination: published=%+v drafts=%+v", publishedPage, draftPage)
	}

	for index := 0; index < 25; index++ {
		id := fmt.Sprintf("pending-%02d", index)
		if err := db.Insert(blogInvitationsCollection, &BlogInvitation{Id: id, BlogId: blog.Id, TokenHash: id, Status: BlogInvitationPending,
			CreatedById: "owner", CreatedByNick: "owner", CreatedOn: stamp, ExpiresOn: stamp.Add(24 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 9; index++ {
		id := fmt.Sprintf("revoked-%02d", index)
		if err := db.Insert(blogInvitationsCollection, &BlogInvitation{Id: id, BlogId: blog.Id, TokenHash: id, Status: BlogInvitationRevoked,
			CreatedById: "owner", CreatedByNick: "owner", CreatedOn: stamp.Add(time.Hour), ExpiresOn: stamp.Add(24 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	invitationPath := "/v1/blogs/" + blog.Id + "/invitations"
	firstInvites := decodeInvitationPage(t, getBody(t, get, invitationPath, true))
	secondInvites := decodeInvitationPage(t, getBody(t, get, invitationPath+"?offset=20", true))
	if len(firstInvites.Items) != 20 || len(secondInvites.Items) != 5 {
		t.Fatalf("pending invitation page sizes = %d and %d", len(firstInvites.Items), len(secondInvites.Items))
	}
	inviteIDs := make([]string, 0, 25)
	for _, invitation := range append(firstInvites.Items, secondInvites.Items...) {
		if invitation.Status != BlogInvitationPending {
			t.Fatalf("non-pending invitation was paginated: %+v", invitation)
		}
		inviteIDs = append(inviteIDs, invitation.Id)
	}
	assertNoDuplicateIDs(t, inviteIDs)

	for _, path := range []string{"/?offset=-1", "/v1/search/articles?q=needle&offset=nope", invitationPath + "?offset=-3", "/v1/blogs/" + blog.Id + "/articles?published=maybe"} {
		authenticated := strings.HasPrefix(path, "/v1/blogs/")
		response, _ := get(path, authenticated)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid offset %q returned %d", path, response.StatusCode)
		}
	}
}

func TestLegacyArticleFieldsAreMigratedBeforePaginationIndexes(t *testing.T) {
	db := newBlogTestDB(t)
	if err := db.CreateCollection("articles"); err != nil {
		t.Fatal(err)
	}
	createdOn := time.Now().Add(-time.Hour).UTC()
	if err := db.Insert("articles", JSON{
		"id": "legacy-pagination", "author_id": "legacy-owner", "author_nick": "legacy",
		"title": "Legacy", "url": "legacy", "createdOn": createdOn, "publishOn": createdOn, "published": true,
	}); err != nil {
		t.Fatal(err)
	}
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	NewApi("", "test", db, fs, nil)
	article := &Article{}
	if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{"id": "legacy-pagination"}}, article); err != nil {
		t.Fatal(err)
	}
	if article.UpdatedOn.IsZero() {
		t.Fatal("legacy updatedOn was not backfilled before index creation")
	}
}

type searchPagePayload struct {
	Items      []PublicArticleSearchResult `json:"items"`
	HasMore    bool                        `json:"hasMore"`
	NextOffset int                         `json:"nextOffset"`
}

func getBody(t *testing.T, get func(string, bool) (*http.Response, []byte), path string, authenticated bool) []byte {
	t.Helper()
	response, body := get(path, authenticated)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", path, response.StatusCode, body)
	}
	return body
}

func decodeSearchPage(t *testing.T, body []byte) searchPagePayload {
	t.Helper()
	result := searchPagePayload{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func decodeInvitationPage(t *testing.T, body []byte) ListPage[*PublicBlogInvitation] {
	t.Helper()
	result := ListPage[*PublicBlogInvitation]{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func paginationHTMLIDs(pattern *regexp.Regexp, body []byte) []string {
	matches := pattern.FindAllSubmatch(body, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, string(match[1]))
	}
	return result
}

func assertNoDuplicateIDs(t *testing.T, ids []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate paginated ID %q in %v", id, ids)
		}
		seen[id] = true
	}
}
