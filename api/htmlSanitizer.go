package api

import (
	"strings"

	"golang.org/x/net/html"
)

func HtmlSanitizer(rawHtml string) string {

	blackListTags := map[string]bool{
		"script": true,
		"iframe": false,
		"object": false,
		"embed":  false,
		"link":   true,
		"form":   true,
		"meta":   true,
	}

	blackListAttributes := map[string]bool{
		"style":      true,
		"xlink:href": true,
	}

	result := &strings.Builder{}

	z := html.NewTokenizer(strings.NewReader(rawHtml))

	for {
		token := z.Next()
		if token == html.ErrorToken {
			break
		}
		switch token {
		case html.StartTagToken:

			token := z.Token()
			startName := token.Data

			if !blackListTags[startName] {
				// Sanitize attributes:
				newTokens := []html.Attribute{}
				for _, a := range token.Attr {
					if strings.HasPrefix(a.Key, "on") {
						continue
					}
					if strings.HasPrefix(strings.ToLower(a.Val), "javascript:") {
						continue
					}
					if blackListAttributes[a.Key] {
						continue
					}
					newTokens = append(newTokens, a)
				}
				token.Attr = newTokens
				result.WriteString(token.String())
				continue
			}

			// Sanitize tags
			for {
				token := z.Next()
				if token == html.ErrorToken {
					break
				}
				if token == html.EndTagToken {
					tagName, _ := z.TagName()
					endName := string(tagName)
					if endName == startName {
						break
					}
				}
			}
		default:
			result.Write(z.Raw())
		}
	}

	return result.String()
}
