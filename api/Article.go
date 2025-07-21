package api

import (
	"encoding/json"
	"html/template" // TODO: this dependency should NOT be here
	"strings"
	"time"
)

type Article struct {
	Id string `json:"id"` // todo: this is part of persistence layer/logic

	ArticleUserFields

	CreatedOn     time.Time `json:"createdOn"`
	PublishOn     time.Time `json:"publishOn"`
	Published     bool      `json:"published"` // todo: use date to program publishment in the future
	AuthorId      string    `json:"author_id"`
	AuthorNick    string    `json:"author_nick"`
	AuthorPicture string    `json:"author_picture"`

	Stats ArticleStats `json:"stats"`

	Banned bool `json:"banned"`
}

type ArticleStats struct {
	Views uint64 `json:"views"` // total number of views
	//	Impressions uint64 `json:"impressions"` // total number of impressions
}

type ArticleUserFields struct {
	Url            string        `json:"url"`
	Title          string        `json:"title"`
	Content        Content       `json:"content"`
	ContentHTML    template.HTML `json:"content_html"`    // it works like a cache
	ContentSummary template.HTML `json:"content_summary"` // it works like a cache for the summary
	Tags           []string      `json:"tags"`
}

type Content struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ArticleShort struct {
	Id        string       `json:"id"`
	Title     string       `json:"title"`
	Url       string       `json:"url"`
	Published bool         `json:"published"`
	Stats     ArticleStats `json:"stats"`
	Tags      []string     `json:"tags"`
	CreatedOn time.Time    `json:"created_on"`
}

func Slug(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ":", "-")

	return s
}
