package api

import (
	"strings"

	"golang.org/x/net/html"
)

var allowedSummaryTokens = map[string]bool{
	// format
	"b":      true,
	"strong": true,
	"i":      true,
	"em":     true,
	"mark":   true,
	"small":  true,
	"del":    true,
	"ins":    true,
	"sub":    true,
	"sup":    true,
	// link
	"a": true,
}

func HtmlSummary(s string, nwords int) string {

	const ellipsis = "..."

	result := &strings.Builder{}

	z := html.NewTokenizer(strings.NewReader(s))

	stack := []string{}
	lastWord := false

	for {
		if nwords <= 0 {
			result.WriteString(ellipsis)
			break
		}
		token := z.Next()
		if token == html.ErrorToken {
			break
		}
		switch token {
		case html.TextToken:

			words := []string{}
			for _, word := range strings.Split(string(z.Text()), " ") {
				if nwords <= 0 {
					break
				}
				word = strings.TrimSpace(word)
				if word == "" {
					continue
				}
				nwords--
				words = append(words, word)
			}

			if lastWord && len(words) > 0 {
				result.WriteString(" ")
			}

			lastWord = len(words) > 0 || lastWord

			result.Write([]byte(strings.Join(words, " ")))
		case html.StartTagToken:
			t := z.Token()
			name := t.Data
			if allowedSummaryTokens[name] {
				if lastWord {
					result.WriteString(" ")
					lastWord = false
				}
				result.WriteString(t.String())
				stack = append(stack, name)
			}
		case html.SelfClosingTagToken:
			// do nothing
		case html.EndTagToken:
			t := z.Token()
			name := t.Data
			if len(stack) > 0 && stack[len(stack)-1] == name {
				result.WriteString(t.String())
				stack = stack[:len(stack)-1]
			}
		}
	}

	for i := len(stack) - 1; i >= 0; i-- {
		result.Write([]byte("</" + stack[i] + ">"))
	}

	return result.String()
}

func TextSummary(s string) string {
	s = html2text(s)
	words := []string{}

	MAX := 50

	n := 0
	for _, w := range strings.Split(s, " ") {
		if n > MAX {
			words = append(words, "...")
			break
		}
		if w == "" {
			continue
		}
		words = append(words, strings.TrimSpace(w))
		n++
	}

	return strings.Join(words, " ")
}
