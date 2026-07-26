package api

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

func sanitizeTranslatedHTML(rawHTML string) string {
	allowedTags := map[string]bool{
		"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"ul": true, "ol": true, "li": true, "div": true, "span": true, "figure": true, "img": true,
		"figcaption": true, "blockquote": true, "pre": true, "code": true, "table": true, "thead": true,
		"tbody": true, "tr": true, "th": true, "td": true, "br": true, "strong": true, "b": true,
		"em": true, "i": true, "u": true, "mark": true, "a": true, "input": true,
	}
	result := &strings.Builder{}
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}
		token := tokenizer.Token()
		switch tokenType {
		case html.StartTagToken, html.SelfClosingTagToken:
			if !allowedTags[token.Data] {
				continue
			}
			token.Attr = translatedHTMLAttributes(token.Data, token.Attr)
			result.WriteString(token.String())
		case html.EndTagToken:
			if allowedTags[token.Data] {
				result.WriteString(token.String())
			}
		case html.TextToken:
			result.WriteString(token.String())
		}
	}
	return result.String()
}

func translatedHTMLAttributes(tag string, attributes []html.Attribute) []html.Attribute {
	result := []html.Attribute{}
	for _, attribute := range attributes {
		key := strings.ToLower(attribute.Key)
		allowed := key == "class" || key == "lang"
		switch tag {
		case "a":
			allowed = allowed || key == "href" || key == "target" || key == "rel"
		case "img":
			allowed = allowed || key == "src" || key == "alt"
		case "input":
			allowed = key == "type" || key == "checked" || key == "disabled"
		case "td", "th":
			allowed = allowed || key == "colspan" || key == "rowspan"
		}
		if !allowed {
			continue
		}
		if key == "href" || key == "src" {
			parsed, err := url.Parse(strings.TrimSpace(attribute.Val))
			if err != nil || (parsed.IsAbs() && parsed.Scheme != "http" && parsed.Scheme != "https") {
				continue
			}
		}
		result = append(result, attribute)
	}
	return result
}
