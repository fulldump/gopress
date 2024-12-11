package api

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"sort"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
)

func RenderUserTag(w http.ResponseWriter, ctx context.Context) {
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
		list = append(list, article)
		userNick = article.AuthorNick
	}) // todo: handle error properly
	if len(list) == 0 {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("User not found"))
		return
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].PublishOn.Unix() > list[j].PublishOn.Unix()
	})

	t := box.GetBoxContext(ctx).Action.GetAttribute("template").(*template.Template)
	err := t.ExecuteTemplate(w, "", map[string]any{
		"tag":      tag,
		"userNick": userNick,
		"articles": list,
	})

	if err != nil {
		log.Println("Error rendering home:", err.Error())
	}
}
