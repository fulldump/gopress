package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/inceptiondb"
)

const (
	ArticleLockTokenHeader = "X-Edit-Lock-Token"
	articleLockTTL         = 90 * time.Second
	articleHeartbeatAfter  = 30 * time.Second
)

type ArticleLockLease struct {
	Token                 string `json:"token,omitempty"`
	ExpiresAt             int64  `json:"expiresAt"`
	ServerTime            int64  `json:"serverTime"`
	TTLSeconds            int64  `json:"ttlSeconds"`
	HeartbeatAfterSeconds int64  `json:"heartbeatAfterSeconds"`
}

type AcquireArticleLockRequest struct {
	ClientId string `json:"clientId"`
}

func AcquireArticleLock(ctx context.Context, w http.ResponseWriter, r *http.Request) (*ArticleLockLease, error) {
	article, _, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return nil, err
	}
	input := &AcquireArticleLockRequest{}
	if err := decodeBlogJSON(w, r, input); err != nil {
		return nil, invalidArticle("clientId es obligatorio")
	}
	input.ClientId = strings.TrimSpace(input.ClientId)
	if input.ClientId == "" || len(input.ClientId) > 200 {
		return nil, invalidArticle("clientId es obligatorio y no puede superar 200 caracteres")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	now := time.Now().UTC().UnixMilli()
	auth := glueauth.GetAuth(ctx)
	lock := &ArticleEditLock{TokenHash: hashArticleLockToken(token), HolderUserId: auth.User.ID,
		HolderNick: auth.User.Nick, ClientId: input.ClientId, AcquiredAt: now, RenewedAt: now,
		ExpiresAt: now + articleLockTTL.Milliseconds()}
	base := articleIdentityFilter(article)
	filters := []JSON{
		mergeFilter(base, JSON{"_edit_lock.expires_at": JSON{"$lt": now + 1}}),
		mergeFilter(base, JSON{"_edit_lock": nil}),
	}
	for _, filter := range filters {
		patched := &Article{}
		matched, err := GetInceptionClient(ctx).PatchOne("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{"_edit_lock": lock}}, patched)
		if err != nil {
			return nil, err
		}
		if matched {
			return articleLockLease(token, lock.ExpiresAt, now), nil
		}
	}
	current := &Article{}
	_ = GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{Filter: articleIdentityFilter(article)}, current)
	return nil, articleLocked(current.EditLock, now)
}

func RenewArticleLock(ctx context.Context) (*ArticleLockLease, error) {
	article, _, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return nil, err
	}
	token, tokenHash, err := requestArticleLockToken(ctx)
	if err != nil {
		return nil, err
	}
	if err := revalidateArticleAccess(ctx, article); err != nil {
		return nil, err
	}
	now := time.Now().UTC().UnixMilli()
	expiresAt := now + articleLockTTL.Milliseconds()
	filter := mergeFilter(articleIdentityFilter(article), JSON{
		"_edit_lock.token_hash": tokenHash, "_edit_lock.holder_user_id": glueauth.GetAuth(ctx).User.ID,
		"_edit_lock.expires_at": JSON{"$gt": now},
	})
	patched := &Article{}
	matched, err := GetInceptionClient(ctx).PatchOne("articles", inceptiondb.PatchQuery{Filter: filter,
		Patch: JSON{"_edit_lock": JSON{"renewed_at": now, "expires_at": expiresAt}}}, patched)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, articleLockLost()
	}
	return articleLockLease(token, expiresAt, now), nil
}

func ReleaseArticleLock(ctx context.Context, w http.ResponseWriter) error {
	article, _, _, err := FindAccessibleArticle(ctx)
	if err != nil {
		return err
	}
	_, tokenHash, err := requestArticleLockToken(ctx)
	if err != nil {
		return err
	}
	if err := revalidateArticleAccess(ctx, article); err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	filter := mergeFilter(articleIdentityFilter(article), JSON{
		"_edit_lock.token_hash": tokenHash, "_edit_lock.holder_user_id": glueauth.GetAuth(ctx).User.ID,
		"_edit_lock.expires_at": JSON{"$gt": now},
	})
	patched := &Article{}
	matched, err := GetInceptionClient(ctx).PatchOne("articles", inceptiondb.PatchQuery{Filter: filter, Patch: JSON{"_edit_lock": nil}}, patched)
	if err != nil {
		return err
	}
	if !matched {
		return articleLockLost()
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func verifyArticleLease(ctx context.Context, article *Article) error {
	_, tokenHash, err := requestArticleLockToken(ctx)
	if err != nil {
		return err
	}
	auth := glueauth.GetAuth(ctx)
	now := time.Now().UTC().UnixMilli()
	if article.EditLock == nil || article.EditLock.HolderUserId != auth.User.ID || article.EditLock.ExpiresAt <= now ||
		subtle.ConstantTimeCompare([]byte(article.EditLock.TokenHash), []byte(tokenHash)) != 1 {
		return articleLockLost()
	}
	return revalidateArticleAccess(ctx, article)
}

func articleMutationFilter(ctx context.Context, article *Article, revision uint64) (JSON, error) {
	if err := verifyArticleLease(ctx, article); err != nil {
		return nil, err
	}
	_, tokenHash, _ := requestArticleLockToken(ctx)
	now := time.Now().UTC().UnixMilli()
	return mergeFilter(articleIdentityFilter(article), JSON{
		"revision": revision, "_edit_lock.token_hash": tokenHash,
		"_edit_lock.holder_user_id": glueauth.GetAuth(ctx).User.ID,
		"_edit_lock.expires_at":     JSON{"$gt": now},
	}), nil
}

func decodeArticleMutationRevision(ctx context.Context, r *http.Request) (uint64, error) {
	input := struct {
		Revision *uint64 `json:"revision"`
	}{}
	if err := decodeBlogJSON(box.GetResponse(ctx), r, &input); err != nil {
		return 0, invalidArticle("revision es obligatoria")
	}
	if input.Revision == nil {
		return 0, invalidArticle("revision es obligatoria")
	}
	return *input.Revision, nil
}

func articleIdentityFilter(article *Article) JSON {
	return JSON{"id": article.Id, "blog_id": article.BlogId, "createdOn": article.CreatedOn}
}

func mergeFilter(base JSON, extra JSON) JSON {
	result := JSON{}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range extra {
		result[key] = value
	}
	return result
}

func requestArticleLockToken(ctx context.Context) (string, string, error) {
	token := strings.TrimSpace(box.GetRequest(ctx).Header.Get(ArticleLockTokenHeader))
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return "", "", articleLockLost()
	}
	return token, hashArticleLockToken(token), nil
}

func hashArticleLockToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func articleLockLease(token string, expiresAt, now int64) *ArticleLockLease {
	return &ArticleLockLease{Token: token, ExpiresAt: expiresAt, ServerTime: now,
		TTLSeconds: int64(articleLockTTL.Seconds()), HeartbeatAfterSeconds: int64(articleHeartbeatAfter.Seconds())}
}

func publicArticleLock(lock *ArticleEditLock) *PublicArticleEditLock {
	if lock == nil || lock.ExpiresAt <= time.Now().UTC().UnixMilli() {
		return nil
	}
	return &PublicArticleEditLock{HolderNick: lock.HolderNick, ExpiresAt: lock.ExpiresAt}
}

func articleLocked(lock *ArticleEditLock, now int64) HttpError {
	public := publicArticleLock(lock)
	if public == nil {
		public = &PublicArticleEditLock{ExpiresAt: now}
	}
	return HttpError{Status: http.StatusLocked, Title: "Article Locked", Code: "article_locked",
		Description: "El artículo está siendo editado por otra persona", Details: JSON{"lock": public}}
}

func articleLockLost() HttpError {
	return HttpError{Status: http.StatusConflict, Title: "Edit Lock Lost", Code: "edit_lock_lost", Description: "El bloqueo de edición ya no es válido"}
}

func articleRevisionConflict(current uint64) HttpError {
	return HttpError{Status: http.StatusConflict, Title: "Article Revision Conflict", Code: "revision_conflict",
		Description: "El artículo cambió desde la última lectura", Details: JSON{"currentRevision": current}}
}

func articleMutationFailure(ctx context.Context, article *Article, revision uint64) HttpError {
	current := &Article{}
	if err := GetInceptionClient(ctx).FindOne("articles", inceptiondb.FindQuery{Filter: articleIdentityFilter(article)}, current); err == nil && current.Revision != revision {
		return articleRevisionConflict(current.Revision)
	}
	return articleLockLost()
}

func invalidArticle(description string) HttpError {
	return HttpError{Status: http.StatusBadRequest, Title: "Invalid Article", Description: description}
}
