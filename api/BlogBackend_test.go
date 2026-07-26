package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
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

func TestBlogFileUploadAssociation(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := box.Box2Http(NewApi("", "test", db, fs, nil))
	client := apitest.NewWithHandler(handler)
	authHeader := `{"user":{"id":"owner","nick":"ana"}}`
	requestAsOwner := func(method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication, authHeader)
	}
	created := requestAsOwner(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "files-blog", "name": "Files",
	}).Do()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create blog: %d %s", created.StatusCode, created.BodyString())
	}
	blogId := created.BodyJsonMap()["id"].(string)

	upload := func(userHeader, targetBlog, filename, mime string, content []byte) (*httptest.ResponseRecorder, map[string]any) {
		t.Helper()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
		header.Set("Content-Type", mime)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/v1/files?blogId="+url.QueryEscape(targetBlog), body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set(glueauth.XGlueAuthentication, userHeader)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		payload := map[string]any{}
		_ = json.Unmarshal(response.Body.Bytes(), &payload)
		return response, payload
	}

	imageResponse, imagePayload := upload(authHeader, blogId, "header.png", "image/png", []byte("png"))
	if imageResponse.Code != http.StatusCreated {
		t.Fatalf("upload image: %d %s", imageResponse.Code, imageResponse.Body.String())
	}
	fileURL := imagePayload["file"].(map[string]any)["url"].(string)
	file := &File{}
	if err := db.FindOne("files", inceptiondb.FindQuery{Filter: JSON{"id": strings.TrimPrefix(fileURL, "/files/")}}, file); err != nil {
		t.Fatal(err)
	}
	if file.BlogId != blogId || file.AuthorId != "owner" || file.Mime != "image/png" {
		t.Fatalf("file association is incomplete: %+v", file)
	}
	if response := requestAsOwner(http.MethodPatch, "/v1/blogs/"+blogId).
		WithBodyJson(JSON{"header_image": fileURL}).Do(); response.StatusCode != http.StatusOK {
		t.Fatalf("set uploaded header: %d %s", response.StatusCode, response.BodyString())
	}

	other := requestAsOwner(http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "other-files", "name": "Other",
	}).Do().BodyJsonMap()
	if response := requestAsOwner(http.MethodPatch, "/v1/blogs/"+other["id"].(string)).
		WithBodyJson(JSON{"header_image": fileURL}).Do(); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-blog header accepted: %d %s", response.StatusCode, response.BodyString())
	}
	outsiderHeader := `{"user":{"id":"outsider","nick":"cora"}}`
	if response, _ := upload(outsiderHeader, blogId, "foreign.png", "image/png", []byte("png")); response.Code != http.StatusForbidden {
		t.Fatalf("outsider uploaded to blog: %d %s", response.Code, response.Body.String())
	}
	textResponse, textPayload := upload(authHeader, blogId, "notes.txt", "text/plain", []byte("not an image"))
	if textResponse.Code != http.StatusCreated {
		t.Fatalf("upload attachment: %d %s", textResponse.Code, textResponse.Body.String())
	}
	textURL := textPayload["file"].(map[string]any)["url"].(string)
	if response := requestAsOwner(http.MethodPatch, "/v1/blogs/"+blogId).
		WithBodyJson(JSON{"header_image": textURL}).Do(); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-image header accepted: %d %s", response.StatusCode, response.BodyString())
	}
}

func TestFileBlogMigrationIsIdempotent(t *testing.T) {
	db := newBlogTestDB(t)
	for _, collection := range []string{"files", "articles"} {
		if err := db.CreateCollection(collection); err != nil {
			t.Fatal(err)
		}
	}
	createdOn := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := db.Insert("files", &File{
		Id: "legacy-file", AuthorId: "file-owner", AuthorNick: "file-author",
		Name: "legacy.png", Mime: "image/png", CreatedOn: createdOn,
	}); err != nil {
		t.Fatal(err)
	}
	if err := InitializeBlogCollections(db); err != nil {
		t.Fatal(err)
	}
	first, err := MigrateFileBlogIds(db)
	if err != nil {
		t.Fatal(err)
	}
	if first.FilesPatched != 1 || first.BlogsCreated != 1 {
		t.Fatalf("unexpected first migration: %+v", first)
	}
	file := &File{}
	if err := db.FindOne("files", inceptiondb.FindQuery{Filter: JSON{"id": "legacy-file"}}, file); err != nil {
		t.Fatal(err)
	}
	if file.BlogId != deterministicPersonalBlogId("file-owner") {
		t.Fatalf("legacy file mapped to %q", file.BlogId)
	}
	second, err := MigrateFileBlogIds(db)
	if err != nil {
		t.Fatal(err)
	}
	if second.FilesPatched != 0 || second.BlogsCreated != 0 || second.FilesUnchanged != 1 {
		t.Fatalf("file migration is not idempotent: %+v", second)
	}
}

func TestBlogMigrationRerunRepairsPartialState(t *testing.T) {
	db := newBlogTestDB(t)
	if err := db.CreateCollection("articles"); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, article := range []JSON{
		{"id": "old", "author_id": "user-1", "author_nick": "old-nick", "createdOn": oldTime, "url": "old-url"},
		{"id": "new", "author_id": "user-1", "author_nick": "new-nick", "createdOn": oldTime.Add(time.Hour), "url": "new-url"},
	} {
		if err := db.Insert("articles", article); err != nil {
			t.Fatal(err)
		}
	}
	first, err := MigrateBlogs(db)
	if err != nil {
		t.Fatalf("first migration: %v (%+v)", err, first)
	}
	blogId := deterministicPersonalBlogId("user-1")
	blog := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, blog); err != nil {
		t.Fatal(err)
	}
	if blog.Slug != "new-nick" || blog.OwnerNick != "new-nick" {
		t.Fatalf("migration did not use most recent nick: %+v", blog)
	}

	patched := &Article{}
	if matched, err := db.PatchOne("articles", inceptiondb.PatchQuery{Filter: JSON{"id": "new"}, Patch: JSON{
		"blog_slug": "wrong", "revision": 0,
	}}, patched); err != nil || !matched {
		t.Fatalf("damage article: matched=%v err=%v", matched, err)
	}
	if matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{"id": blogId}, Patch: JSON{
		"slug": "", "accent_color": "",
	}}, blog); err != nil || !matched {
		t.Fatalf("damage blog: matched=%v err=%v", matched, err)
	}
	if matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": blogId, "members_revision": blog.MembersRevision,
	}, Patch: JSON{"members": []BlogMember{}, "members_revision": blog.MembersRevision + 1}}, blog); err != nil || !matched {
		t.Fatalf("damage owner membership: matched=%v err=%v", matched, err)
	}

	second, err := MigrateBlogs(db)
	if err != nil {
		t.Fatalf("repair migration: %v (%+v)", err, second)
	}
	if second.BlogsRepaired != 1 || second.MembershipsCreated != 1 || second.ArticlesPatched != 1 {
		t.Fatalf("unexpected repair report: %+v", second)
	}
	article := &Article{}
	if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{"id": "new"}}, article); err != nil {
		t.Fatal(err)
	}
	if article.Revision != 1 || article.BlogSlug != "new-nick" || article.Url != "new-url" || !article.CreatedOn.Equal(oldTime.Add(time.Hour)) {
		t.Fatalf("article was not coherently repaired: %+v", article)
	}
	third, err := MigrateBlogs(db)
	if err != nil {
		t.Fatal(err)
	}
	if third.BlogsCreated != 0 || third.BlogsRepaired != 0 || third.MembershipsCreated != 0 || third.ArticlesPatched != 0 {
		t.Fatalf("migration is not idempotent: %+v", third)
	}
}

func TestBlogMigrationRejectsEmptyAuthor(t *testing.T) {
	db := newBlogTestDB(t)
	if err := db.CreateCollection("articles"); err != nil {
		t.Fatal(err)
	}
	createdOn := time.Now().UTC()
	if err := db.Insert("articles", JSON{"id": "orphan", "author_id": "", "createdOn": createdOn}); err != nil {
		t.Fatal(err)
	}
	report, err := MigrateBlogs(db)
	if err != nil || len(report.Errors) != 1 || report.ArticlesSkipped != 1 {
		t.Fatalf("orphan article was not safely skipped: report=%+v err=%v", report, err)
	}
	orphan := &migrationArticle{}
	if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{"id": "orphan"}}, orphan); err != nil {
		t.Fatal(err)
	}
	if orphan.BlogId != "" {
		t.Fatalf("orphan article was mapped to blog %q", orphan.BlogId)
	}
}

func TestBlogMigrationSkipsMalformedRowsAndRecoversDates(t *testing.T) {
	db := newBlogTestDB(t)
	if err := db.CreateCollection("articles"); err != nil {
		t.Fatal(err)
	}
	publishedOn := time.Date(2024, 8, 2, 10, 0, 0, 0, time.UTC)
	for _, article := range []JSON{
		{"id": "broken", "author_id": "", "createdOn": publishedOn},
		{"id": "recoverable", "author_id": "author", "author_nick": "writer", "publishOn": publishedOn},
	} {
		if err := db.Insert("articles", article); err != nil {
			t.Fatal(err)
		}
	}
	report, err := MigrateBlogs(db)
	if err != nil {
		t.Fatalf("migration aborted on malformed legacy row: %v (%+v)", err, report)
	}
	if report.ArticlesScanned != 2 || report.ArticlesSkipped != 1 || report.ArticlesPatched != 1 || len(report.Errors) != 1 {
		t.Fatalf("unexpected migration report: %+v", report)
	}
	recovered := &migrationArticle{}
	if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{"id": "recoverable"}}, recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.BlogId != deterministicPersonalBlogId("author") || !recovered.CreatedOn.Equal(publishedOn) {
		t.Fatalf("recoverable article was not migrated: %+v", recovered)
	}
	broken := &migrationArticle{}
	if err := db.FindOne("articles", inceptiondb.FindQuery{Filter: JSON{"id": "broken"}}, broken); err != nil {
		t.Fatal(err)
	}
	if broken.BlogId != "" {
		t.Fatalf("malformed row was modified: %+v", broken)
	}
}

func TestBlogAccessInvitationsAndArticleLocks(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := apitest.NewWithHandler(NewApi("", "test", db, fs, nil))
	requestAs := func(userId, nick, method, path string) *apitest.Request {
		return client.Request(method, path).WithHeader(glueauth.XGlueAuthentication,
			`{"user":{"id":"`+userId+`","nick":"`+nick+`"}}`)
	}

	createBlog := requestAs("owner", "ana", http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "team-blog", "name": "Team", "language": "en", "accent_color": "#12ABEF",
	}).Do()
	if createBlog.StatusCode != http.StatusCreated {
		t.Fatalf("create blog: %d %s", createBlog.StatusCode, createBlog.BodyString())
	}
	blogId := createBlog.BodyJsonMap()["id"].(string)
	if strings.Contains(createBlog.BodyString(), "members") {
		t.Fatalf("create blog leaked embedded members: %s", createBlog.BodyString())
	}
	persisted := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, persisted); err != nil {
		t.Fatal(err)
	}
	if !hasCanonicalOwnerMember(persisted) || persisted.MembersRevision != 1 {
		t.Fatalf("blog was not inserted with its owner atomically: %+v", persisted)
	}
	forged := &BlogMember{Id: deterministicBlogMemberId(blogId, "outsider"), BlogId: blogId,
		UserId: "outsider", UserNick: "cora", Role: BlogRoleOwner, CreatedOn: time.Now().UTC()}
	if matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": blogId, "members_revision": persisted.MembersRevision,
	}, Patch: JSON{"members": append(persisted.Members, *forged), "members_revision": persisted.MembersRevision + 1}}, persisted); err != nil || !matched {
		t.Fatalf("forge embedded owner: matched=%v err=%v", matched, err)
	}
	if response := requestAs("outsider", "cora", http.MethodGet, "/v1/blogs/"+blogId).Do(); response.StatusCode != http.StatusForbidden {
		t.Fatalf("membership role overrode authoritative owner: %d", response.StatusCode)
	}
	for _, response := range []*apitest.Response{
		requestAs("owner", "ana", http.MethodGet, "/v1/blogs").Do(),
		requestAs("owner", "ana", http.MethodGet, "/v1/blogs/"+blogId).Do(),
	} {
		if response.StatusCode != http.StatusOK || strings.Contains(response.BodyString(), "members") || strings.Contains(response.BodyString(), "outsider") {
			t.Fatalf("blog response leaked embedded members: %d %s", response.StatusCode, response.BodyString())
		}
	}
	inviteResponse := requestAs("owner", "ana", http.MethodPost, "/v1/blogs/"+blogId+"/invitations").Do()
	if inviteResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create invite: %d %s", inviteResponse.StatusCode, inviteResponse.BodyString())
	}
	invite := inviteResponse.BodyJsonMap()
	token := strings.TrimPrefix(invite["inviteUrl"].(string), "/admin/#/invite/")
	if token == "" || strings.Contains(inviteResponse.BodyString(), "token_hash") {
		t.Fatalf("invitation leaked hash or omitted token URL: %s", inviteResponse.BodyString())
	}
	accepted := requestAs("editor", "bea", http.MethodPost, "/v1/invitations/accept").
		WithBodyJson(JSON{"token": token}).Do()
	if accepted.StatusCode != http.StatusOK {
		t.Fatalf("accept invite: %d %s", accepted.StatusCode, accepted.BodyString())
	}
	for _, response := range []*apitest.Response{
		accepted,
		requestAs("editor", "bea", http.MethodGet, "/v1/blogs").Do(),
		requestAs("editor", "bea", http.MethodGet, "/v1/blogs/"+blogId).Do(),
	} {
		if response.StatusCode != http.StatusOK || strings.Contains(response.BodyString(), "members") || strings.Contains(response.BodyString(), `"user_nick":"bea"`) {
			t.Fatalf("editor blog response leaked embedded members: %d %s", response.StatusCode, response.BodyString())
		}
	}
	members := requestAs("owner", "ana", http.MethodGet, "/v1/blogs/"+blogId+"/members").Do()
	if members.StatusCode != http.StatusOK || !strings.Contains(members.BodyString(), `"user_id":"owner"`) || !strings.Contains(members.BodyString(), `"user_id":"editor"`) {
		t.Fatalf("owner member API changed: %d %s", members.StatusCode, members.BodyString())
	}
	if response := requestAs("editor", "bea", http.MethodPatch, "/v1/blogs/"+blogId).
		WithBodyJson(JSON{"name": "Forbidden"}).Do(); response.StatusCode != http.StatusForbidden {
		t.Fatalf("editor changed config: %d", response.StatusCode)
	}

	created := requestAs("editor", "bea", http.MethodPost, "/v1/blogs/"+blogId+"/articles").
		WithBodyJson(JSON{"title": "Shared article"}).Do()
	if created.StatusCode != http.StatusOK {
		t.Fatalf("editor create article: %d %s", created.StatusCode, created.BodyString())
	}
	article := created.BodyJsonMap()
	articleId := article["id"].(string)
	root := "/v1/blogs/" + blogId + "/articles/" + articleId
	secondBlog := requestAs("owner", "ana", http.MethodPost, "/v1/blogs").WithBodyJson(JSON{
		"slug": "other-blog", "name": "Other",
	}).Do().BodyJsonMap()
	wrongRoot := "/v1/blogs/" + secondBlog["id"].(string) + "/articles/" + articleId
	if response := requestAs("owner", "ana", http.MethodGet, wrongRoot).Do(); response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-blog article lookup returned %d", response.StatusCode)
	}

	ownerLock := requestAs("owner", "ana", http.MethodPost, root+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "owner-tab"}).Do()
	if ownerLock.StatusCode != http.StatusOK {
		t.Fatalf("acquire lock: %d %s", ownerLock.StatusCode, ownerLock.BodyString())
	}
	ownerToken := ownerLock.BodyJsonMap()["token"].(string)
	if response := requestAs("owner", "ana", http.MethodPost, root+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "owner-tab"}).Do(); response.StatusCode != http.StatusLocked || errorCode(response) != "article_locked" {
		t.Fatalf("same client replaced active token: %d %s", response.StatusCode, response.BodyString())
	}
	if response := requestAs("editor", "bea", http.MethodPost, root+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "editor-tab"}).Do(); response.StatusCode != http.StatusLocked {
		t.Fatalf("editor bypassed lock: %d", response.StatusCode)
	}
	visible := requestAs("editor", "bea", http.MethodGet, root).Do()
	for _, secret := range []string{"token_hash", "holder_user_id", "client_id", ownerToken} {
		if strings.Contains(visible.BodyString(), secret) {
			t.Fatalf("article response exposed %q: %s", secret, visible.BodyString())
		}
	}
	replacement := "A"
	if ownerToken[0] == 'A' {
		replacement = "B"
	}
	badToken := replacement + ownerToken[1:]
	if response := requestAs("owner", "ana", http.MethodPost, root+"/edit-lock/renew").
		WithHeader(ArticleLockTokenHeader, badToken).Do(); response.StatusCode != http.StatusConflict || errorCode(response) != "edit_lock_lost" {
		t.Fatalf("wrong-token renew: %d %s", response.StatusCode, response.BodyString())
	}
	if response := requestAs("owner", "ana", http.MethodPost, root+"/edit-lock/renew").
		WithHeader(ArticleLockTokenHeader, ownerToken).Do(); response.StatusCode != http.StatusOK {
		t.Fatalf("renew lock: %d %s", response.StatusCode, response.BodyString())
	}

	patchBody := JSON{"revision": article["revision"], "title": "Revised", "content": JSON{
		"type": "editorjs", "data": JSON{"blocks": []JSON{{"type": "paragraph", "data": JSON{"text": "<script>x</script>safe"}}}},
	}}
	patchedResponse := requestAs("owner", "ana", http.MethodPatch, root).
		WithHeader(ArticleLockTokenHeader, ownerToken).WithBodyJson(patchBody).Do()
	if patchedResponse.StatusCode != http.StatusOK {
		t.Fatalf("patch article: %d %s", patchedResponse.StatusCode, patchedResponse.BodyString())
	}
	patchedArticle := patchedResponse.BodyJsonMap()
	if strings.Contains(patchedArticle["content_html"].(string), "<script") {
		t.Fatal("server-generated HTML was not sanitized")
	}
	if stale := requestAs("owner", "ana", http.MethodPatch, root).
		WithHeader(ArticleLockTokenHeader, ownerToken).WithBodyJson(patchBody).Do(); stale.StatusCode != http.StatusConflict || errorCode(stale) != "revision_conflict" {
		t.Fatalf("stale revision: %d %s", stale.StatusCode, stale.BodyString())
	}
	if released := requestAs("owner", "ana", http.MethodPost, root+"/edit-lock/release").
		WithHeader(ArticleLockTokenHeader, ownerToken).Do(); released.StatusCode != http.StatusNoContent {
		t.Fatalf("release lock: %d %s", released.StatusCode, released.BodyString())
	}
	if secondRelease := requestAs("owner", "ana", http.MethodPost, root+"/edit-lock/release").
		WithHeader(ArticleLockTokenHeader, ownerToken).Do(); secondRelease.StatusCode != http.StatusConflict {
		t.Fatalf("second release: %d", secondRelease.StatusCode)
	}

	editorLock := requestAs("editor", "bea", http.MethodPost, root+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "editor-tab"}).Do()
	editorToken := editorLock.BodyJsonMap()["token"].(string)
	expired := &Article{}
	if matched, err := db.PatchOne("articles", inceptiondb.PatchQuery{Filter: JSON{"id": articleId}, Patch: JSON{
		"_edit_lock": JSON{"expires_at": time.Now().Add(-time.Second).UnixMilli()},
	}}, expired); err != nil || !matched {
		t.Fatalf("expire lock: matched=%v err=%v", matched, err)
	}
	if publish := requestAs("editor", "bea", http.MethodPost, root+"/publish").
		WithHeader(ArticleLockTokenHeader, editorToken).
		WithBodyJson(JSON{"revision": patchedArticle["revision"]}).Do(); publish.StatusCode != http.StatusConflict {
		t.Fatalf("expired lease published: %d %s", publish.StatusCode, publish.BodyString())
	}
	freshLock := requestAs("editor", "bea", http.MethodPost, root+"/edit-lock/acquire").
		WithBodyJson(JSON{"clientId": "editor-tab-2"}).Do()
	if freshLock.StatusCode != http.StatusOK {
		t.Fatalf("acquire after expiry: %d %s", freshLock.StatusCode, freshLock.BodyString())
	}
	freshToken := freshLock.BodyJsonMap()["token"].(string)
	firstPublish := requestAs("editor", "bea", http.MethodPost, root+"/publish").
		WithHeader(ArticleLockTokenHeader, freshToken).
		WithBodyJson(JSON{"revision": patchedArticle["revision"]}).Do()
	if firstPublish.StatusCode != http.StatusOK {
		t.Fatalf("first publish: %d %s", firstPublish.StatusCode, firstPublish.BodyString())
	}
	publishOn := firstPublish.BodyJsonMap()["publishOn"]
	secondPublish := requestAs("editor", "bea", http.MethodPost, root+"/publish").
		WithHeader(ArticleLockTokenHeader, freshToken).
		WithBodyJson(JSON{"revision": firstPublish.BodyJsonMap()["revision"]}).Do()
	if secondPublish.StatusCode != http.StatusOK || secondPublish.BodyJsonMap()["publishOn"] != publishOn {
		t.Fatalf("republish changed publishOn: first=%v second=%v", publishOn, secondPublish.BodyJsonMap()["publishOn"])
	}

	removed := requestAs("owner", "ana", http.MethodDelete, "/v1/blogs/"+blogId+"/members/editor").Do()
	if removed.StatusCode != http.StatusOK {
		t.Fatalf("remove editor: %d %s", removed.StatusCode, removed.BodyString())
	}
	if denied := requestAs("editor", "bea", http.MethodGet, root).Do(); denied.StatusCode != http.StatusForbidden {
		t.Fatalf("removed editor retained article access: %d", denied.StatusCode)
	}
	reused := requestAs("editor", "bea", http.MethodPost, "/v1/invitations/accept").WithBodyJson(JSON{"token": token}).Do()
	if reused.StatusCode != http.StatusGone {
		t.Fatalf("accepted invitation was reusable: %d %s", reused.StatusCode, reused.BodyString())
	}
	blogAfterRemoval := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blogId}}, blogAfterRemoval); err != nil {
		t.Fatal(err)
	}
	if member := findBlogMember(blogAfterRemoval.Members, "editor"); member != nil {
		t.Fatalf("used invitation recreated removed member: %+v", member)
	}
	if ownerRemoval := requestAs("owner", "ana", http.MethodDelete, "/v1/blogs/"+blogId+"/members/owner").Do(); ownerRemoval.StatusCode != http.StatusForbidden {
		t.Fatalf("owner removal returned %d", ownerRemoval.StatusCode)
	}
}

func TestEmbeddedBlogMemberMigrationDropsLegacyCollectionAndReruns(t *testing.T) {
	db := newBlogTestDB(t)
	if err := InitializeBlogCollections(db); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCollection(legacyBlogMembersCollection); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	blog := &Blog{Id: "legacy-blog", Slug: "legacy-blog", Name: "Legacy", Language: BlogLanguageES,
		AccentColor: "#2563EB", OwnerId: "owner", OwnerNick: "ana", CreatedOn: now, UpdatedOn: now}
	if err := db.Insert(blogsCollection, blog); err != nil {
		t.Fatal(err)
	}
	for _, member := range []BlogMember{
		{Id: "legacy-owner", BlogId: blog.Id, UserId: "forged", Role: BlogRoleOwner, CreatedOn: now},
		{Id: "legacy-editor", BlogId: blog.Id, UserId: "editor", UserNick: "bea", Role: BlogRoleEditor, CreatedOn: now},
	} {
		if err := db.Insert(legacyBlogMembersCollection, &member); err != nil {
			t.Fatal(err)
		}
	}
	report, err := MigrateEmbeddedBlogMembers(db)
	if err != nil {
		t.Fatal(err)
	}
	if !report.LegacyCollectionFound || report.LegacyRowsScanned != 2 || report.EditorsMigrated != 1 || report.BlogsPatched != 1 {
		t.Fatalf("unexpected migration report: %+v", report)
	}
	if _, err := db.GetCollection(legacyBlogMembersCollection); !errors.Is(err, inceptiondb.ErrorNotFound) {
		t.Fatalf("legacy collection remains visible: %v", err)
	}
	persisted := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blog.Id}}, persisted); err != nil {
		t.Fatal(err)
	}
	if !hasCanonicalOwnerMember(persisted) || findBlogMember(persisted.Members, "editor") == nil || findBlogMember(persisted.Members, "forged") != nil {
		t.Fatalf("members were not canonicalized: %+v", persisted.Members)
	}
	rerun, err := MigrateEmbeddedBlogMembers(db)
	if err != nil {
		t.Fatal(err)
	}
	if rerun.LegacyCollectionFound || rerun.BlogsPatched != 0 {
		t.Fatalf("missing-collection rerun changed data: %+v", rerun)
	}
}

func TestEmbeddedBlogMemberMigrationRejectsDuplicateEditors(t *testing.T) {
	db := newBlogTestDB(t)
	if err := InitializeBlogCollections(db); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCollection(legacyBlogMembersCollection); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	blog := &Blog{Id: "duplicate-blog", Slug: "duplicate-blog", Name: "Duplicate", Language: BlogLanguageES,
		AccentColor: "#2563EB", OwnerId: "owner", CreatedOn: now, UpdatedOn: now}
	if err := insertBlogWithOwner(db, blog); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"duplicate-a", "duplicate-b"} {
		if err := db.Insert(legacyBlogMembersCollection, &BlogMember{Id: id, BlogId: blog.Id, UserId: "editor", Role: BlogRoleEditor, CreatedOn: now}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := MigrateEmbeddedBlogMembers(db); err == nil || !strings.Contains(err.Error(), "dangerous duplicate") {
		t.Fatalf("duplicate migration error = %v", err)
	}
	if _, err := db.GetCollection(legacyBlogMembersCollection); err != nil {
		t.Fatalf("failed migration dropped source collection: %v", err)
	}
}

func TestBlogMemberCASRejectsStaleArrayReplacement(t *testing.T) {
	db := newBlogTestDB(t)
	if err := InitializeBlogCollections(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	blog := &Blog{Id: "cas-blog", Slug: "cas-blog", Name: "CAS", Language: BlogLanguageES,
		AccentColor: "#2563EB", OwnerId: "owner", CreatedOn: now, UpdatedOn: now}
	if err := insertBlogWithOwner(db, blog); err != nil {
		t.Fatal(err)
	}
	first := &Blog{}
	second := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blog.Id}}, first); err != nil {
		t.Fatal(err)
	}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blog.Id}}, second); err != nil {
		t.Fatal(err)
	}
	first.Members = append(first.Members, BlogMember{Id: deterministicBlogMemberId(blog.Id, "editor-a"), BlogId: blog.Id,
		UserId: "editor-a", Role: BlogRoleEditor, CreatedOn: now})
	if matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": blog.Id, "members_revision": first.MembersRevision,
	}, Patch: JSON{"members": first.Members, "members_revision": first.MembersRevision + 1}}, &Blog{}); err != nil || !matched {
		t.Fatalf("first CAS: matched=%v err=%v", matched, err)
	}
	second.Members = append(second.Members, BlogMember{Id: deterministicBlogMemberId(blog.Id, "editor-b"), BlogId: blog.Id,
		UserId: "editor-b", Role: BlogRoleEditor, CreatedOn: now})
	if matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": blog.Id, "members_revision": second.MembersRevision,
	}, Patch: JSON{"members": second.Members, "members_revision": second.MembersRevision + 1}}, &Blog{}); err != nil || matched {
		t.Fatalf("stale CAS: matched=%v err=%v", matched, err)
	}
	if _, _, err := ensureEditorMembership(db, second, "editor-b", "editor", now); err != nil {
		t.Fatal(err)
	}
	persisted := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blog.Id}}, persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Members) != 3 || findBlogMember(persisted.Members, "editor-a") == nil || findBlogMember(persisted.Members, "editor-b") == nil || !hasCanonicalOwnerMember(persisted) {
		t.Fatalf("CAS retry lost members: revision=%d members=%+v", persisted.MembersRevision, persisted.Members)
	}
}

func errorCode(response *apitest.Response) string {
	body := response.BodyJsonMap()
	errorBody, _ := body["error"].(map[string]any)
	code, _ := errorBody["code"].(string)
	return code
}

func newBlogTestDB(t *testing.T) *inceptiondb.Client {
	t.Helper()
	store := database.NewDatabase(&database.Config{Dir: t.TempDir()})
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Stop() })
	server := httptest.NewServer(box.Box2Http(inceptionapi.Build(service.NewService(store), "", "test")))
	t.Cleanup(server.Close)
	return inceptiondb.NewClient(inceptiondb.Config{Base: server.URL + "/v1"})
}
