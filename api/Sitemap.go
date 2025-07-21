package api

import (
	"context"
	"encoding/xml"
	"net/http"

	"gopress/inceptiondb"
)

func Sitemap(ctx context.Context, w http.ResponseWriter) {

	w.Header().Set("content-type", "text/xml; charset=UTF-8")
	// Begin XML
	w.Write([]byte(xml.Header))
	w.Write([]byte(`<urlset xmlns="http://www.google.com/schemas/sitemap/0.9">` + "\n"))

	// Collect users
	users := map[string]*Article{}
	tags := map[string]*Article{}

	// Article pages
	params := inceptiondb.FindQuery{
		Limit: 9999,
		Filter: JSON{
			"published": true,
			"$ne":       JSON{"banned": true},
		},
	}
	GetInceptionClient(ctx).FindAll("articles", params, func(article *Article) {
		w.Write([]byte(`    <url>
        <loc>https://gopress.org/user/` + article.AuthorNick + `/article/` + article.Url + `</loc>
        <lastmod>` + article.PublishOn.UTC().Format("2006-01-02") + `</lastmod>
        <changefreq>weekly</changefreq>
        <priority>0.6</priority>
    </url>`))

		{
			lastArticle, exist := users[article.AuthorId]
			if !exist || article.PublishOn.After(lastArticle.PublishOn) {
				users[article.AuthorId] = article
			}
		}

		for _, tag := range article.Tags {
			lastArticle, exist := tags[tag]
			if !exist || article.PublishOn.After(lastArticle.PublishOn) {
				tags[tag] = article
			}
		}
	})

	// User pages
	for _, article := range users {
		w.Write([]byte(`    <url>
        <loc>https://gopress.org/user/` + article.AuthorNick + `</loc>
        <lastmod>` + article.PublishOn.UTC().Format("2006-01-02") + `</lastmod>
        <changefreq>daily</changefreq>
        <priority>0.4</priority>
    </url>`))
	}

	// Tag pages
	for tag, lastArticle := range tags {
		w.Write([]byte(`    <url>
        <loc>https://gopress.org/tag/` + tag + `</loc>
        <lastmod>` + lastArticle.PublishOn.UTC().Format("2006-01-02") + `</lastmod>
        <changefreq>hourly</changefreq>
        <priority>0.2</priority>
    </url>`))
	}

	// End XML
	w.Write([]byte(`</urlset>`))

}
