package api

import (
	"fmt"
	"testing"
)

func TestHtml2text(t *testing.T) {

	text := html2text(`

<h1>My title</h1>

<p>
	Hello, this is my <strong>first</strong> article.
</p>

`)

	fmt.Println(removeSpaces(text))
}
