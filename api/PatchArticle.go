package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/fulldump/box"
	"golang.org/x/net/html"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func PatchArticle(r *http.Request, ctx context.Context) any {

	db := GetInceptionClient(ctx)
	auth := glueauth.GetAuth(ctx)

	articleId := box.GetUrlParameter(ctx, "articleId")

	article := &Article{}
	err := db.FindOne("articles", inceptiondb.FindQuery{
		Filter: JSON{
			"id":        articleId,
			"author_id": auth.User.ID,
		},
	}, article)
	if err != nil {
		log.Println("patch article: db find:", err.Error())
		return JSON{
			"error": "could not read from data storage",
		}
	}

	oldTitle := article.Title

	err = json.NewDecoder(r.Body).Decode(&article.ArticleUserFields)
	if err != nil {
		log.Println("patch article: json decode:", err.Error())
		return JSON{
			"error": "could not read JSON",
		}
	}

	// If title has changed, update slug
	if oldTitle != article.Title {
		article.Url = Slug(article.Title)
	}

	contentHtml, err := editorjs2HTML(article.Content.Data)
	if err != nil {
		return JSON{
			"error": "invalid payload to transform from editorjs 2 html",
		}
	}
	article.ContentHTML = template.HTML(contentHtml)

	_, err = db.Patch("articles", inceptiondb.PatchQuery{
		Filter: JSON{
			"id":        articleId,
			"author_id": auth.User.ID,
		},
		Patch: article,
	})
	if err != nil {
		log.Println("patch article: db patch:", err.Error())
		return JSON{
			"error": "could not write to data storage",
		}
	}

	return article
}

type EditorJs struct {
	Blocks []*Block `json:"blocks"`
	// time
	// version
}

type Block struct {
	Id   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func editorjs2HTML(data []byte) (string, error) {

	e := &EditorJs{}
	err := json.Unmarshal(data, &e)
	if err != nil {
		return "", err
	}

	result := &strings.Builder{}

	for _, block := range e.Blocks {

		switch block.Type {
		case "header":
			header := struct {
				Level int
				Text  string
			}{}
			json.Unmarshal(block.Data, &header) // todo: handle error properly
			fmt.Fprintf(result, "<h%d>%s</h%d>\n", header.Level, header.Text, header.Level)

		case "paragraph":
			header := struct {
				Text string
			}{}
			json.Unmarshal(block.Data, &header) // todo: handle error properly
			fmt.Fprintf(result, "<p>%s</p>\n", header.Text)

		case "image":
			image := struct {
				Caption string
				File    struct {
					Url string
				}
				Stretched      bool
				WithBackground bool
				WithBorder     bool
			}{}
			json.Unmarshal(block.Data, &image) // todo: handle error properly

			class := []string{"image"}
			if image.WithBorder {
				class = append(class, "image-withborder")
			}
			if image.Stretched {
				class = append(class, "image-stretched")
			}
			if image.WithBackground {
				class = append(class, "image-withbackground")
			}

			fmt.Fprintf(result, `<figure class="%s" style="text-align: center;"><div class="border"><img src="%s" alt="%s"></div><figcaption>%s</figcaption></figure>`+"\n",
				strings.Join(class, " "), image.File.Url, image.Caption, image.Caption)

			// todo: escape: html.EscapeString()

		case "list":
			list := struct {
				Style string
				Items []string
			}{}
			json.Unmarshal(block.Data, &list) // todo: handle error properly
			tag := "ul"
			if list.Style == "ordered" {
				tag = "ol"
			}
			fmt.Fprintf(result, "<%s>\n", tag)
			for _, item := range list.Items {
				fmt.Fprintf(result, "<li>%s</li>\n", item)
			}
			fmt.Fprintf(result, "</%s>\n", tag)

		case "checklist":
			checklist := struct {
				Items []struct {
					Checked bool
					Text    string
				}
			}{}
			json.Unmarshal(block.Data, &checklist) // todo: handle error properly

			fmt.Fprintf(result, `<div class="checklist">`)
			for _, item := range checklist.Items {
				fmt.Fprintf(result, `<div class="checklist-item">`)
				if item.Checked {
					fmt.Fprintf(result, `<input type="checkbox" checked disabled>`)
				} else {
					fmt.Fprintf(result, `<input type="checkbox" disabled>`)
				}
				fmt.Fprint(result, " ", item.Text)
				fmt.Fprintf(result, `</div>`)
			}
			fmt.Fprintf(result, `</div>`)

		case "quote":
			quote := struct {
				Caption   string
				Text      string
				Alignment string
			}{}
			json.Unmarshal(block.Data, &quote) // todo: handle error properly

			fmt.Fprintf(result, `<figure class="quote">
    <blockquote>
        <p>%s</p>
    </blockquote>
    <figcaption><cite>%s</cite></figcaption>
</figure>`, quote.Text, quote.Caption)

		case "warning":
			warning := struct {
				Title   string
				Message string
			}{}
			json.Unmarshal(block.Data, &warning) // todo: handle error properly

			fmt.Fprintf(result, `<figure class="warning">
    <figcaption>⚠️ %s</figcaption>
    <blockquote>
        <p>%s</p>
    </blockquote>
</figure>`, warning.Title, warning.Message)

		case "delimiter":

			fmt.Fprintf(result, `<hr>`+"\n")

		case "linkTool":
			// todo

		case "table":
			table := struct {
				WithHeadings bool
				Content      [][]string
			}{}
			json.Unmarshal(block.Data, &table) // todo: handle error properly

			fmt.Fprintf(result, `<table class="table">`+"\n")
			for i, row := range table.Content {
				fmt.Fprintf(result, `<tr>`+"\n")
				for _, col := range row {
					if i == 0 && table.WithHeadings {
						fmt.Fprintf(result, `<th>%s</th>`+"\n", col)
					} else {
						fmt.Fprintf(result, `<td>%s</td>`+"\n", col)
					}
				}
				fmt.Fprintf(result, `</tr>`+"\n")
			}
			fmt.Fprintf(result, `</table>`+"\n")

		case "code":
			code := struct {
				Code string
			}{}
			json.Unmarshal(block.Data, &code) // todo: handle error properly
			fmt.Fprintf(result, `<code class="code-block">%s</code>`+"\n", html.EscapeString(code.Code))

		case "raw":
			raw := struct {
				Html string
			}{}
			json.Unmarshal(block.Data, &raw) // todo: handle error properly
			result.WriteString(raw.Html)

		case "attaches":
			attaches := struct {
				Title string
				File  struct {
					Url string
				}
			}{}
			json.Unmarshal(block.Data, &attaches) // todo: handle error properly

			fmt.Fprintf(result, `<div class="attaches"><a href="%s" target="_blank">📎 %s</a></div>`, attaches.File.Url, attaches.Title)

		case "embed":
			embed := struct {
				Caption string
				Embed   string
				Height  int
				Service string
				Source  string
				Width   int
			}{}
			json.Unmarshal(block.Data, &embed) // todo: handle error properly

			if embed.Service == "twitch-video" || embed.Service == "twitch-channel" {
				embed.Embed += "&parent=gopress.org"
			}

			fmt.Fprintf(result, `<figure style="text-align: center;">`)
			fmt.Fprintf(result, `<iframe style="width:100%%;" height="%d" frameborder="0" allowfullscreen="" src="%s" class="embed-tool__content" frameborder="0" allow="autoplay; encrypted-media" allowfullscreen></iframe>`+"\n", embed.Height, embed.Embed)
			fmt.Fprintf(result, `<figcaption>`)
			fmt.Fprintf(result, embed.Caption)
			fmt.Fprintf(result, `</figcaption>`)
			fmt.Fprintf(result, `</figure>`)
		}
	}

	return result.String(), nil
}
