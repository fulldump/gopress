package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
	"gopress/templates"
)

func RenderArticle(w http.ResponseWriter, ctx context.Context) error {

	userNick := box.GetUrlParameter(ctx, "userNick")
	articleUrl := box.GetUrlParameter(ctx, "articleUrl")

	filter := JSON{
		"author_nick": userNick,
		"url":         articleUrl,
		"published":   true,
		"$ne":         JSON{"banned": true},
	}

	db := GetInceptionClient(ctx)

	article := &Article{}
	err := db.FindOne("articles", inceptiondb.FindQuery{
		Filter: filter,
	}, article)
	if err != nil {
		return HttpError{
			Status:      http.StatusNotFound,
			Title:       "Article Not Found",
			Description: "Article was not found",
		}
	}

	// TODO: preprocess html tags to remove

	max_words := 15
	words := strings.SplitN(article.Title, " ", max_words)
	words_trail := ""
	if len(words) >= max_words {
		words = words[0 : max_words-1]
		words_trail = "..."
	}
	title := "@" + article.AuthorNick + ": " + strings.Join(words, " ") + words_trail
	selfUrl := `https://gopress.org/user/` + url.PathEscape(article.AuthorId)

	content := string(article.ContentHTML)
	content = html2text(content)
	content = removeSpaces(content)
	description := content
	max_description := 150
	if len(description) > max_description {
		description = content[0:max_description] + "..."
	}

	err = templates.Execute(ctx, "article", w, map[string]any{
		"article":        article,
		"og_title":       title,
		"og_url":         selfUrl,
		"og_image":       article.AuthorPicture,
		"og_description": description,
	})

	if err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}

	go func() {
		// Naive way to have visits counter
		defer func() {
			if r := recover(); r != nil {
				log.Println("RenderArticle: db patch:", r)
			}
		}()
		db.Patch("articles", inceptiondb.PatchQuery{
			Filter: filter,
			Patch: JSON{
				"stats": JSON{
					"views": article.Stats.Views + 1,
				},
			},
		})
	}()

	return nil
}
