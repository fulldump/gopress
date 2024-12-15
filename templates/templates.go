package templates

import (
	"context"
	_ "embed"
	"fmt"
	"html/template"

	"github.com/fulldump/box"
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

type Templates interface {
	Get(name string) *template.Template
}

type EmbeddedTemplates struct {
	list map[string]*template.Template
}

func NewEmbeddedTemplates() (*EmbeddedTemplates, error) {

	texts := map[string]string{
		"home":    Home,
		"article": Article,
		"user":    User,
		"tag":     Tag,
		"error":   Error,
	}
	list := map[string]*template.Template{}

	for name, text := range texts {
		t, err := template.New("").Parse(text)
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

func Inject(t Templates) box.I {
	return func(next box.H) box.H {
		return func(ctx context.Context) {
			ctx = Set(ctx, t)
			next(ctx)
		}
	}
}
