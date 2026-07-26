package templates

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/fulldump/box"

	"gopress/i18n"
)

//go:embed home.gohtml
var Home string

//go:embed article.gohtml
var Article string

//go:embed user.gohtml
var User string

//go:embed tag.gohtml
var Tag string

//go:embed error.gohtml
var Error string

//go:embed bookmarks.gohtml
var Bookmarks string

//go:embed blogs.gohtml
var Blogs string

//go:embed blog.gohtml
var Blog string

type Templates interface {
	Get(name string) *template.Template
}

type EmbeddedTemplates struct {
	list map[string]*template.Template
}

func NewEmbeddedTemplates() (*EmbeddedTemplates, error) {

	texts := map[string]string{
		"home":      Home,
		"article":   Article,
		"user":      User,
		"tag":       Tag,
		"error":     Error,
		"bookmarks": Bookmarks,
		"blogs":     Blogs,
		"blog":      Blog,
	}
	list := map[string]*template.Template{}

	for name, text := range texts {
		t, err := template.New("").Funcs(template.FuncMap{
			"t": func(locale i18n.Locale, key string) string { return i18n.Translate(locale, key) },
			"json": func(value any) template.JS {
				encoded, _ := json.Marshal(value)
				return template.JS(encoded)
			},
		}).Parse(text)
		if err != nil {
			return nil, fmt.Errorf("error parsing template %s: %w", name, err)
		}
		list[name] = t
	}

	return &EmbeddedTemplates{list}, nil
}

func (e *EmbeddedTemplates) Get(name string) *template.Template {
	return e.list[name]
}

func Set(ctx context.Context, t Templates) context.Context {
	return context.WithValue(ctx, "templates", t)
}

func Get(ctx context.Context) Templates {
	return ctx.Value("templates").(Templates)
}

func GetByName(ctx context.Context, name string) *template.Template {
	return Get(ctx).Get(name)
}

type JSON = map[string]any

var GlobalData = JSON{
	"headTrailCode": template.HTML(""),
}

func Execute(ctx context.Context, name string, w http.ResponseWriter, data JSON) error {
	merged := JSON{}
	for k, v := range GlobalData {
		merged[k] = v
	}
	for k, v := range data {
		merged[k] = v
	}
	locale := i18n.GetLocale(ctx)
	merged["locale"] = locale
	merged["messages"] = i18n.ClientMessages(locale)
	return GetByName(ctx, name).Execute(w, merged)
}

func Inject(t Templates) box.I {
	return func(next box.H) box.H {
		return func(ctx context.Context) {
			ctx = Set(ctx, t)
			next(ctx)
		}
	}
}
