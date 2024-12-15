package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderUser(w http.ResponseWriter, ctx context.Context) error {
	// todo: limit page size to 10
	// todo: sort by date DESC

	userNick := box.GetUrlParameter(ctx, "userNick")

	params := inceptiondb.FindQuery{
		Limit: 1000,
		Filter: JSON{
			"author_nick": userNick,
			"published":   true,
		},
	}
	list := []*Article{}
	GetInceptionClient(ctx).FindAll("articles", params, func(article *Article) {
		list = append(list, article)
	}) // todo: handle error properly
	if len(list) == 0 {
		return HttpError{
			Status:      http.StatusNotFound,
			Title:       "Blog Not Found",
			Description: fmt.Sprintf("El blog '%s' todavía no existe", userNick),
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].PublishOn.Unix() > list[j].PublishOn.Unix()
	})

	err := templates.GetByName(ctx, "user").ExecuteTemplate(w, "", map[string]any{
		"userNick": userNick,
		"articles": list,
	})
	if err != nil {
		log.Println("Error rendering home:", err.Error())
	}

	return nil
}
