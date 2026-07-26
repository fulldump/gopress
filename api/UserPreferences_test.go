package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fulldump/apitest"
	"github.com/fulldump/box"

	"gopress/filestorage/localfilestore"
	"gopress/glueauth"
)

func TestLocaleUsesBrowserCookieAndUserPreference(t *testing.T) {
	db := newBlogTestDB(t)
	fs, err := localfilestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := box.Box2Http(NewApi("", "test", db, fs, nil))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9,es;q=0.8")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), `<html lang="en">`) || !strings.Contains(string(body), "Latest posts") {
		t.Fatalf("browser locale was ignored: %s", body)
	}
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/", nil)
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), `<html lang="zh">`) || !strings.Contains(string(body), "最新文章") ||
		!strings.Contains(string(body), `rel="canonical" href="https://gopress.org/?lang=zh"`) ||
		!strings.Contains(string(body), `hreflang="zh" href="https://gopress.org/?lang=zh"`) {
		t.Fatalf("Chinese browser locale was ignored: %s", body)
	}
	response, err = http.Get(server.URL + "/blogs?lang=zh")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), `rel="canonical" href="https://gopress.org/blogs?lang=zh"`) ||
		!strings.Contains(string(body), `hreflang="zh" href="https://gopress.org/blogs?lang=zh"`) {
		t.Fatalf("Chinese blogs metadata is missing: %s", body)
	}

	client := apitest.NewWithHandler(handler)
	auth := `{"user":{"id":"locale-user","nick":"reader"}}`
	put := client.Request(http.MethodPut, "/v1/preferences").WithHeader(glueauth.XGlueAuthentication, auth).
		WithBodyJson(JSON{"locale": "zh-CN"}).Do()
	if put.StatusCode != http.StatusOK || put.BodyJsonMap()["locale"] != "zh" {
		t.Fatalf("save preference: %d %s", put.StatusCode, put.BodyString())
	}
	get := client.Request(http.MethodGet, "/v1/preferences").WithHeader(glueauth.XGlueAuthentication, auth).Do()
	if get.StatusCode != http.StatusOK || get.BodyJsonMap()["locale"] != "zh" {
		t.Fatalf("load preference: %d %s", get.StatusCode, get.BodyString())
	}
}
