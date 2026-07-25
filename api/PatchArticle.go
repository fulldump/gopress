package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fulldump/box"
	"golang.org/x/net/html"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

func PatchArticle(r *http.Request, ctx context.Context) any {
	db := GetInceptionClient(ctx)
	auth := glueauth.GetAuth(ctx)
	article, _, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return err
	}
	input := struct {
		Title    *string  `json:"title,omitempty"`
		Content  *Content `json:"content,omitempty"`
		Revision *uint64  `json:"revision"`
	}{}
	if err = decodeBlogJSON(box.GetResponse(ctx), r, &input); err != nil {
		log.Println("patch article: json decode:", err)
		return invalidArticle("No se pudo leer el JSON")
	}
	if input.Revision == nil {
		return invalidArticle("revision es obligatoria")
	}
	if input.Title == nil && input.Content == nil {
		return invalidArticle("title o content son obligatorios")
	}
	if *input.Revision != article.Revision {
		return articleRevisionConflict(article.Revision)
	}
	if input.Title != nil {
		article.Title = *input.Title
	}
	if input.Content != nil {
		article.Content = *input.Content
	}
	if article.Content.Type != "editorjs" {
		return invalidArticle("content.type debe ser editorjs")
	}
	contentHTML, err := editorjs2HTML(article.Content.Data)
	if err != nil {
		return invalidArticle("El contenido del artículo no es válido")
	}
	article.ContentHTML = template.HTML(HtmlSanitizer(contentHTML))
	filter, err := articleMutationFilter(ctx, article, *input.Revision)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	patched := &Article{}
	matched, err := db.PatchOne("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{
		"title": article.Title, "content": article.Content, "content_html": article.ContentHTML,
		"revision": *input.Revision + 1, "updatedOn": now,
		"updated_by_id": auth.User.ID, "updated_by_nick": auth.User.Nick,
	}}, patched)
	if err != nil {
		log.Println("patch article: db patch:", err.Error())
		return HttpError{Status: http.StatusInternalServerError, Title: "Article Save Failed", Description: "No se pudo guardar el artículo"}
	}
	if !matched {
		return articleMutationFailure(ctx, article, *input.Revision)
	}
	setArticlePublicURL(patched)
	return patched
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
				Items []json.RawMessage
			}{}
			json.Unmarshal(block.Data, &list) // todo: handle error properly
			tag := "ul"
			if list.Style == "ordered" {
				tag = "ol"
			}
			fmt.Fprintf(result, "<%s>\n", tag)
			if err := renderEditorListItems(result, list.Items, tag); err != nil {
				return "", err
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

			sanitized := HtmlSanitizer(raw.Html)

			result.WriteString(sanitized)

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

	return HtmlSanitizer(result.String()), nil
}

func renderEditorListItems(result *strings.Builder, items []json.RawMessage, tag string) error {
	for _, raw := range items {
		text := ""
		children := []json.RawMessage{}
		if err := json.Unmarshal(raw, &text); err != nil {
			item := struct {
				Content string
				Items   []json.RawMessage
			}{}
			if err := json.Unmarshal(raw, &item); err != nil {
				return err
			}
			text = item.Content
			children = item.Items
		}
		fmt.Fprintf(result, "<li>%s", text)
		if len(children) > 0 {
			fmt.Fprintf(result, "<%s>", tag)
			if err := renderEditorListItems(result, children, tag); err != nil {
				return err
			}
			fmt.Fprintf(result, "</%s>", tag)
		}
		fmt.Fprint(result, "</li>\n")
	}
	return nil
}

func safeEditorURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	if parsed.IsAbs() && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	if !parsed.IsAbs() && !strings.HasPrefix(parsed.Path, "/") {
		return ""
	}
	return value
}
