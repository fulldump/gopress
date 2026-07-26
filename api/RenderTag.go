package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/fulldump/box"

	"gopress/i18n"
	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderTag(w http.ResponseWriter, ctx context.Context) error {
	// todo: limit page size to 10
	// todo: sort by date DESC

	tag := box.GetUrlParameter(ctx, "tag")

	params := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"tags":      tag,
			"published": true,
			"$ne":       JSON{"banned": true},
		},
	}
	list := []*Article{}
	publicBlogs := map[string]*Blog{}
	_ = GetInceptionClient(ctx).FindAll(blogsCollection, inceptiondb.FindQuery{Limit: maxBlogs, Filter: JSON{"$ne": JSON{"banned": true}}}, func(blog *Blog) { publicBlogs[blog.Id] = blog })
	preferred := contentLanguageFromLocale(string(i18n.GetLocale(ctx)))
	GetInceptionClient(ctx).FindAll("articles", params, func(article *Article) {
		blog := publicBlogs[article.BlogId]
		if blog == nil {
			return
		}
		list = append(list, preparePublicArticle(article, blog, preferred))
	}) // todo: handle error properly
	if len(list) == 0 {
		return HttpError{
			Status:      http.StatusNotFound,
			Title:       "Tag Not Found",
			Description: fmt.Sprintf("El tag '%s' todavía no existe", tag),
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].PublishOn.Unix() > list[j].PublishOn.Unix()
	})

	w.Header().Set("X-Robots-Tag", "noindex")
	err := templates.Execute(ctx, "tag", w, map[string]any{
		"tag":      tag,
		"articles": list,
	})

	if err != nil {
		log.Println("Error rendering home:", err.Error())
	}

	return nil
}
