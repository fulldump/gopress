package templates

import _ "embed"

//go:embed home.gohtml
var Home string

//go:embed article.gohtml
var Article string
