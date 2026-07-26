package api

import (
	"context"
	"log"
	"net/http"
	"sort"

	"gopress/i18n"
	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderHome(ctx context.Context, w http.ResponseWriter) {
	// todo: limit page size to 10
	// todo: sort by date DESC

	query := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"published": true,
			"$ne":       JSON{"banned": true},
		},
	}

	list := []*Article{}
	publicBlogs := map[string]*Blog{}
	_ = GetInceptionClient(ctx).FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogs, Filter: JSON{"$ne": JSON{"banned": true}}}, func(blog *Blog) {
		publicBlogs[blog.Id] = blog
	})
	preferred := contentLanguageFromLocale(string(i18n.GetLocale(ctx)))
	GetInceptionClient(ctx).FindAll("articles", query, func(article *Article) {
		blog := publicBlogs[article.BlogId]
		if blog == nil {
			return
		}
		list = append(list, preparePublicArticle(article, blog, preferred))
	}) // todo: handle error properly

	sort.Slice(list, func(i, j int) bool {
		return list[i].PublishOn.Unix() > list[j].PublishOn.Unix()
	})

	err := templates.Execute(ctx, "home", w, map[string]any{
		"articles": list,
	})

	if err != nil {
		log.Println("Error rendering home:", err.Error())
	}
}
