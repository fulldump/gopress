package api

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	blogsCollection           = "blogs"
	blogMembersCollection     = "blog_members"
	blogInvitationsCollection = "blog_invitations"

	maxBlogs             = 1000
	maxBlogMembers       = 1000
	maxBlogInvitations   = 1000
	maxBlogRequestSize   = 64 << 10
	blogInvitationPeriod = 7 * 24 * time.Hour
)

type BlogLanguage string

const (
	BlogLanguageES BlogLanguage = "es"
	BlogLanguageEN BlogLanguage = "en"
)

type BlogRole string

const (
	BlogRoleOwner  BlogRole = "owner"
	BlogRoleEditor BlogRole = "editor"
)

type Blog struct {
	Id                   string       `json:"id"`
	Slug                 string       `json:"slug"`
	Name                 string       `json:"name"`
	Description          string       `json:"description"`
	Language             BlogLanguage `json:"language"`
	AccentColor          string       `json:"accent_color"`
	HeaderImage          string       `json:"header_image"`
	Banned               bool         `json:"banned"`
	BannedOn             *time.Time   `json:"banned_on,omitempty"`
	BannedByArticleId    string       `json:"banned_by_article_id,omitempty"`
	ReviewRequestedOn    *time.Time   `json:"review_requested_on,omitempty"`
	ReviewRequestMessage string       `json:"review_request_message,omitempty"`
	OwnerId              string       `json:"owner_id"`
	OwnerNick            string       `json:"owner_nick"`
	Personal             bool         `json:"personal"`
	LegacyNicks          []string     `json:"legacy_nicks"`
	CreatedOn            time.Time    `json:"createdOn"`
	UpdatedOn            time.Time    `json:"updatedOn"`
}

type BlogMember struct {
	Id        string    `json:"id"`
	BlogId    string    `json:"blog_id"`
	UserId    string    `json:"user_id"`
	UserNick  string    `json:"user_nick"`
	Role      BlogRole  `json:"role"`
	CreatedOn time.Time `json:"createdOn"`
}

type BlogWithRole struct {
	*Blog
	Role BlogRole `json:"role"`
}

var (
	blogSlugPattern    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	accentColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

var reservedBlogSlugs = map[string]struct{}{
	"admin": {}, "admin_old": {}, "api": {}, "v1": {}, "auth": {},
	"blogs": {}, "blog": {}, "user": {}, "users": {}, "tag": {}, "tags": {},
	"files": {}, "styles": {}, "lib": {}, "img": {}, "images": {}, "static": {},
	"assets": {}, "sitemap.xml": {}, "openapi.json": {}, "version": {},
	"robots.txt": {}, "favicon.ico": {}, "bookmarks": {},
}

func IsReservedBlogSlug(slug string) bool {
	_, ok := reservedBlogSlugs[strings.ToLower(slug)]
	return ok
}

func ValidateBlogSlug(slug string) error {
	if len(slug) < 2 || len(slug) > 63 || !blogSlugPattern.MatchString(slug) {
		return errors.New("slug must contain 2 to 63 lowercase ASCII letters, numbers, or hyphens, and cannot start or end with a hyphen")
	}
	if IsReservedBlogSlug(slug) {
		return errors.New("slug is reserved")
	}
	return nil
}

func validateBlogFields(name, description string, language BlogLanguage, accentColor, headerImage string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 120 {
		return errors.New("name is required and cannot exceed 120 characters")
	}
	if utf8.RuneCountInString(description) > 2000 {
		return errors.New("description cannot exceed 2000 characters")
	}
	if language != BlogLanguageES && language != BlogLanguageEN {
		return errors.New("language must be es or en")
	}
	if !accentColorPattern.MatchString(accentColor) {
		return errors.New("accent_color must use #RRGGBB format")
	}
	if headerImage != "" {
		if strings.HasPrefix(headerImage, "/files/") {
			return nil
		}
		parsed, err := url.Parse(headerImage)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("header_image must be empty or an absolute http/https URL")
		}
	}
	return nil
}
