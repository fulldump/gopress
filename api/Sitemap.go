package api

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"sort"
	"time"

	"gopress/inceptiondb"
)

type sitemapURL struct {
	Location  string `xml:"loc"`
	LastMod   string `xml:"lastmod,omitempty"`
	Frequency string `xml:"changefreq,omitempty"`
	Priority  string `xml:"priority,omitempty"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

func Sitemap(ctx context.Context, w http.ResponseWriter) {
	const origin = "https://gopress.org"
	set := sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: []sitemapURL{
		{Location: origin + "/", Frequency: "daily", Priority: "1.0"},
		{Location: origin + "/blogs", Frequency: "daily", Priority: "0.8"},
	}}
	blogs := map[string]*Blog{}
	_ = GetInceptionClient(ctx).FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogs, Filter: JSON{
		"$ne": JSON{"banned": true},
	}}, func(blog *Blog) {
		blogs[blog.Id] = blog
		set.URLs = append(set.URLs, sitemapURL{Location: origin + "/blog/" + url.PathEscape(blog.Slug),
			LastMod: sitemapDate(blog.UpdatedOn), Frequency: "daily", Priority: "0.7"})
	})
	_ = GetInceptionClient(ctx).FindAll("articles", inceptiondb.FindQuery{Limit: 100000, Filter: JSON{
		"published": true, "$ne": JSON{"banned": true},
	}}, func(article *Article) {
		blog := blogs[article.BlogId]
		if blog == nil {
			return
		}
		set.URLs = append(set.URLs, sitemapURL{Location: origin + articlePublicPath(blog.Slug, article.Url),
			LastMod: sitemapDate(article.UpdatedOn), Frequency: "weekly", Priority: "0.6"})
		for _, language := range blogTranslationLanguages(blog) {
			if language == article.SourceLanguage {
				continue
			}
			if translation := articleTranslation(article, language); translation != nil {
				set.URLs = append(set.URLs, sitemapURL{Location: origin + articleTranslationPath(blog.Slug, article.Url, language),
					LastMod: sitemapDate(translation.GeneratedOn), Frequency: "weekly", Priority: "0.5"})
			}
		}
	})
	sort.SliceStable(set.URLs, func(i, j int) bool { return set.URLs[i].Location < set.URLs[j].Location })
	w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
	w.Write([]byte(xml.Header))
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	_ = encoder.Encode(set)
}

func sitemapDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}
