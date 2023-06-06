package api

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

func html2text(s string) string {

	result := &strings.Builder{}

	z := html.NewTokenizer(strings.NewReader(s))

	for {
		token := z.Next()
		if token == html.ErrorToken {
			break
		}
		switch token {
		case html.TextToken:
			result.Write(z.Text())
		}
	}

	return result.String()
}

func removeSpaces(s string) string {
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
}
