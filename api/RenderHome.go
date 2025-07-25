package api

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"sort"

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
	GetInceptionClient(ctx).FindAll("articles", query, func(article *Article) {
		if article.ContentSummary == "" {
			article.ContentSummary = template.HTML(HtmlSummary(string(article.ContentHTML), 50))
		}
		list = append(list, article)
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
