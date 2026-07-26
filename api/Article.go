package api

import (
	"encoding/json"
	"html/template" // TODO: this dependency should NOT be here
	"strings"
	"time"
)

type Article struct {
	Id       string `json:"id"` // todo: this is part of persistence layer/logic
	BlogId   string `json:"blog_id,omitempty"`
	BlogSlug string `json:"blog_slug,omitempty"`

	ArticleUserFields

	CreatedOn     time.Time `json:"createdOn"`
	PublishOn     time.Time `json:"publishOn"`
	Published     bool      `json:"published"` // todo: use date to program publishment in the future
	AuthorId      string    `json:"author_id"`
	AuthorNick    string    `json:"author_nick"`
	AuthorPicture string    `json:"author_picture"`

	Stats ArticleStats `json:"stats"`

	Banned bool `json:"banned"`

	Revision           uint64               `json:"revision"`
	UpdatedOn          time.Time            `json:"updatedOn"`
	UpdatedById        string               `json:"updated_by_id,omitempty"`
	UpdatedByNick      string               `json:"updated_by_nick,omitempty"`
	EditLock           *ArticleEditLock     `json:"_edit_lock,omitempty"`
	PublicUrl          string               `json:"publicUrl,omitempty"`
	SourceLanguage     BlogLanguage         `json:"source_language,omitempty"`
	ContentRevision    uint64               `json:"content_revision,omitempty"`
	Translations       []ArticleTranslation `json:"translations,omitempty"`
	AvailableLanguages []LanguageLink       `json:"available_languages,omitempty"`
}

type ArticleTranslation struct {
	Language              BlogLanguage  `json:"language"`
	Title                 string        `json:"title"`
	ContentHTML           template.HTML `json:"content_html"`
	SourceContentRevision uint64        `json:"source_content_revision"`
	GeneratedOn           time.Time     `json:"generatedOn"`
}

type LanguageLink struct {
	Language BlogLanguage `json:"language"`
	Url      string       `json:"url"`
	Current  bool         `json:"current,omitempty"`
}

type ArticleEditLock struct {
	TokenHash    string `json:"token_hash"`
	HolderUserId string `json:"holder_user_id"`
	HolderNick   string `json:"holder_nick"`
	ClientId     string `json:"client_id"`
	AcquiredAt   int64  `json:"acquired_at"`
	RenewedAt    int64  `json:"renewed_at"`
	ExpiresAt    int64  `json:"expires_at"`
}

type PublicArticleEditLock struct {
	HolderNick string `json:"holderNick"`
	ExpiresAt  int64  `json:"expiresAt"`
}

func (article *Article) MarshalJSON() ([]byte, error) {
	type articleAlias Article
	copy := *article
	copy.EditLock = nil
	return json.Marshal(struct {
		*articleAlias
		EditLock *PublicArticleEditLock `json:"editLock,omitempty"`
	}{articleAlias: (*articleAlias)(&copy), EditLock: publicArticleLock(article.EditLock)})
}

type ArticleStats struct {
	Views uint64 `json:"views"` // total number of views
	//	Impressions uint64 `json:"impressions"` // total number of impressions
}

type ArticleUserFields struct {
	Url            string        `json:"url"`
	AutomaticUrl   bool          `json:"automatic_url,omitempty"`
	Title          string        `json:"title"`
	Content        Content       `json:"content"`
	ContentHTML    template.HTML `json:"content_html"`    // it works like a cache
	ContentSummary template.HTML `json:"content_summary"` // it works like a cache for the summary
	Tags           []string      `json:"tags"`
	TagsMode       string        `json:"tags_mode,omitempty"`
}

type Content struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ArticleShort struct {
	Id        string                 `json:"id"`
	Title     string                 `json:"title"`
	Url       string                 `json:"url"`
	Published bool                   `json:"published"`
	Stats     ArticleStats           `json:"stats"`
	Tags      []string               `json:"tags"`
	CreatedOn time.Time              `json:"created_on"`
	UpdatedOn time.Time              `json:"updated_on"`
	BlogId    string                 `json:"blog_id"`
	BlogSlug  string                 `json:"blog_slug"`
	Revision  uint64                 `json:"revision"`
	EditLock  *PublicArticleEditLock `json:"editLock,omitempty"`
	Languages []BlogLanguage         `json:"languages,omitempty"`
}

func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var result strings.Builder
	lastHyphen := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			result.WriteRune(r)
			lastHyphen = false
		} else if result.Len() > 0 && !lastHyphen {
			result.WriteByte('-')
			lastHyphen = true
		}
	}
	slug := strings.Trim(result.String(), "-")
	if len(slug) > 80 {
		slug = strings.TrimRight(slug[:80], "-")
	}
	if slug == "" {
		return "article"
	}
	return slug
}
