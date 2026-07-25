package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

const (
	blogBanInterval = 10 * time.Minute
	blogBanLookback = 7 * 24 * time.Hour
)

type BlogBanReport struct {
	ArticlesScanned int
	ArticlesMatched int
	BlogsBanned     int
	BlogsUnchanged  int
	Warnings        []string
}

type RequestBlogReviewInput struct {
	Message string `json:"message"`
}

var errInvalidBlogBanSource = errors.New("invalid blog ban source")

func ReconcileBannedBlogs(db *inceptiondb.Client, since time.Time) (*BlogBanReport, error) {
	report := &BlogBanReport{Warnings: []string{}}
	articles := []*Article{}
	filter := JSON{"banned": true}
	if !since.IsZero() {
		filter["$or"] = []JSON{
			{"updatedOn": JSON{"$gt": since}},
			{"publishOn": JSON{"$gt": since}},
			{"createdOn": JSON{"$gt": since}},
		}
	}
	if err := db.FindAll("articles", inceptiondb.FindQuery{Limit: maxBlogMigrationRows, Filter: filter}, func(article *Article) {
		articles = append(articles, article)
	}); err != nil {
		return report, fmt.Errorf("scan banned articles: %w", err)
	}
	report.ArticlesScanned = len(articles)
	seenBlogs := map[string]bool{}
	fatalErrors := []error{}
	for _, article := range articles {
		reconciledOn := article.UpdatedOn
		if reconciledOn.IsZero() {
			reconciledOn = article.PublishOn
		}
		if reconciledOn.IsZero() {
			reconciledOn = article.CreatedOn
		}
		if !since.IsZero() && (reconciledOn.IsZero() || reconciledOn.Before(since)) {
			continue
		}
		report.ArticlesMatched++
		if article.BlogId == "" {
			if article.AuthorId == "" {
				report.Warnings = append(report.Warnings, fmt.Sprintf("banned article %q has no blog_id or author_id", article.Id))
				continue
			}
			article.BlogId = deterministicPersonalBlogId(article.AuthorId)
		}
		if seenBlogs[article.BlogId] {
			continue
		}
		seenBlogs[article.BlogId] = true
		banned, err := banBlogForArticle(db, article)
		if err != nil {
			if errors.Is(err, errInvalidBlogBanSource) {
				report.Warnings = append(report.Warnings, err.Error())
			} else {
				fatalErrors = append(fatalErrors, err)
			}
			continue
		}
		if banned {
			report.BlogsBanned++
		} else {
			report.BlogsUnchanged++
		}
	}
	return report, errors.Join(fatalErrors...)
}

func banBlogForArticle(db *inceptiondb.Client, article *Article) (bool, error) {
	blog := &Blog{}
	if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": article.BlogId}}, blog); err != nil {
		if errors.Is(err, io.EOF) {
			return false, fmt.Errorf("%w: banned article %q references missing blog %q", errInvalidBlogBanSource, article.Id, article.BlogId)
		}
		return false, fmt.Errorf("find blog %q for banned article %q: %w", article.BlogId, article.Id, err)
	}
	if blog.Banned {
		return false, nil
	}
	now := time.Now().UTC()
	patched := &Blog{}
	matched, err := db.PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": blog.Id, "owner_id": blog.OwnerId, "slug": blog.Slug, "$ne": JSON{"banned": true},
	}, Patch: JSON{
		"banned": true, "banned_on": now, "banned_by_article_id": article.Id, "updatedOn": now,
	}}, patched)
	if err != nil {
		return false, fmt.Errorf("ban blog %q: %w", blog.Id, err)
	}
	if !matched {
		current := &Blog{}
		if err := db.FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blog.Id}}, current); err == nil && current.Banned {
			return false, nil
		}
		return false, fmt.Errorf("ban blog %q returned no row", blog.Id)
	}
	return true, nil
}

func RunBlogBanReconciler(db *inceptiondb.Client, stop <-chan struct{}) {
	ticker := time.NewTicker(blogBanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			report, err := ReconcileBannedBlogs(db, time.Now().UTC().Add(-blogBanLookback))
			logBlogBanReport("periodic", report)
			if err != nil {
				log.Printf("blog ban reconciliation failed: %v", err)
				continue
			}
		case <-stop:
			return
		}
	}
}

func logBlogBanReport(run string, report *BlogBanReport) {
	if report.BlogsBanned > 0 || len(report.Warnings) > 0 {
		log.Printf("blog ban reconciliation (%s): scanned=%d matched=%d banned=%d unchanged=%d warnings=%d",
			run, report.ArticlesScanned, report.ArticlesMatched, report.BlogsBanned, report.BlogsUnchanged, len(report.Warnings))
	}
	for index, warning := range report.Warnings {
		if index == 10 {
			log.Printf("blog ban reconciliation: %d additional warnings omitted", len(report.Warnings)-10)
			break
		}
		log.Printf("blog ban reconciliation warning: %s", warning)
	}
}

func RequestBlogReview(ctx context.Context, w http.ResponseWriter, r *http.Request) (*BlogWithRole, error) {
	blog, err := RequireBlogOwner(ctx, box.GetUrlParameter(ctx, "blogId"))
	if err != nil {
		return nil, err
	}
	if !blog.Banned {
		return nil, HttpError{Status: http.StatusConflict, Title: "Blog Is Not Banned", Code: "blog_not_banned", Description: "Este blog no está baneado"}
	}
	if blog.ReviewRequestedOn != nil {
		return nil, HttpError{Status: http.StatusConflict, Title: "Review Already Requested", Code: "review_already_requested", Description: "Ya hay una revisión pendiente"}
	}
	input := &RequestBlogReviewInput{}
	if err := decodeBlogJSON(w, r, input); err != nil {
		return nil, err
	}
	input.Message = strings.TrimSpace(input.Message)
	if len([]rune(input.Message)) < 10 || len([]rune(input.Message)) > 2000 {
		return nil, invalidBlog("review message must contain 10 to 2000 characters")
	}
	now := time.Now().UTC()
	patched := &Blog{}
	matched, err := GetInceptionClient(ctx).PatchOne(blogsCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": blog.Id, "owner_id": glueauth.GetAuth(ctx).User.ID, "banned": true, "updatedOn": blog.UpdatedOn,
	}, Patch: JSON{
		"review_requested_on": now, "review_request_message": input.Message, "updatedOn": now,
	}}, patched)
	if err != nil {
		return nil, blogStorageError("request blog review", err)
	}
	if !matched {
		current := &Blog{}
		if findErr := GetInceptionClient(ctx).FindOne(blogsCollection, inceptiondb.FindQuery{Filter: JSON{"id": blog.Id}}, current); findErr == nil && current.ReviewRequestedOn != nil {
			return nil, HttpError{Status: http.StatusConflict, Title: "Review Already Requested", Code: "review_already_requested", Description: "Ya hay una revisión pendiente"}
		}
		return nil, HttpError{Status: http.StatusConflict, Title: "Blog Changed", Code: "blog_changed", Description: "El blog ha cambiado; vuelve a intentarlo"}
	}
	w.WriteHeader(http.StatusCreated)
	return &BlogWithRole{Blog: patched, Role: BlogRoleOwner}, nil
}

func blogBanned() HttpError {
	return HttpError{Status: http.StatusForbidden, Title: "Blog Banned", Code: "blog_banned", Description: "Este blog está baneado y no puede publicar contenido"}
}
