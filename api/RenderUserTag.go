package api

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderUserTag(w http.ResponseWriter, ctx context.Context) error {
	// todo: limit page size to 10
	// todo: sort by date DESC

	userNick := box.GetUrlParameter(ctx, "userNick")
	tag := box.GetUrlParameter(ctx, "tag")

	params := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"author_nick": userNick,
			"tags":        tag,
			"published":   true,
		},
	}
	list := []*Article{}
	GetInceptionClient(ctx).FindAll("articles", params, func(article *Article) {
		if article.ContentSummary == "" {
			article.ContentSummary = template.HTML(HtmlSummary(string(article.ContentHTML), 50))
		}
		list = append(list, article)
		userNick = article.AuthorNick
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
	err := templates.GetByName(ctx, "user").ExecuteTemplate(w, "", map[string]any{
		"tag":      tag,
		"userNick": userNick,
		"articles": list,
	})

	if err != nil {
		log.Println("Error rendering home:", err.Error())
	}

	return nil
}
