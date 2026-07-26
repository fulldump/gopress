package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/i18n"
	"gopress/inceptiondb"
)

const (
	userPreferencesCollection = "user_preferences"
	localeCookieName          = "gopress_locale"
)

var userPreferenceMu sync.Mutex

type UserPreferences struct {
	Id        string      `json:"id"`
	UserId    string      `json:"user_id"`
	Locale    i18n.Locale `json:"locale"`
	UpdatedOn time.Time   `json:"updatedOn"`
}

func ResolveLocale(next box.H) box.H {
	return func(ctx context.Context) {
		r := box.GetRequest(ctx)
		locale := i18n.Normalize(r.URL.Query().Get("lang"))
		if locale != "" {
			setLocaleCookie(box.GetResponse(ctx), r, locale)
		} else if auth := glueauth.GetAuth(ctx); auth != nil {
			if preferences, err := findUserPreferences(GetInceptionClient(ctx), auth.User.ID); err == nil {
				locale = preferences.Locale
			}
		}
		if locale == "" {
			if cookie, err := r.Cookie(localeCookieName); err == nil {
				locale = i18n.Normalize(cookie.Value)
			}
		}
		if glueauth.GetAuth(ctx) != nil {
			box.GetResponse(ctx).Header().Set("Cache-Control", "private, no-store")
		}
		if locale == "" {
			locale = localeFromAcceptLanguage(r.Header.Get("Accept-Language"))
		}
		if locale == "" {
			locale = i18n.Spanish
		}
		box.GetResponse(ctx).Header().Add("Vary", "Accept-Language, Cookie")
		next(i18n.SetLocale(ctx, locale))
	}
}

func GetUserPreferences(ctx context.Context) (*UserPreferences, error) {
	box.GetResponse(ctx).Header().Set("Cache-Control", "private, no-store")
	auth := glueauth.GetAuth(ctx)
	preferences, err := findUserPreferences(GetInceptionClient(ctx), auth.User.ID)
	if errors.Is(err, io.EOF) {
		return &UserPreferences{UserId: auth.User.ID, Locale: i18n.GetLocale(ctx)}, nil
	}
	return preferences, err
}

func PutUserPreferences(ctx context.Context, w http.ResponseWriter, r *http.Request) (*UserPreferences, error) {
	input := struct {
		Locale string `json:"locale"`
	}{}
	if err := decodeBlogJSON(w, r, &input); err != nil {
		return nil, invalidArticle("locale no válido")
	}
	locale := i18n.Normalize(input.Locale)
	if locale == "" {
		return nil, invalidArticle("locale debe ser es, en o zh")
	}
	auth := glueauth.GetAuth(ctx)
	now := time.Now().UTC()
	preferences := &UserPreferences{Id: userPreferenceId(auth.User.ID), UserId: auth.User.ID, Locale: locale, UpdatedOn: now}
	db := GetInceptionClient(ctx)
	userPreferenceMu.Lock()
	matched, err := db.PatchOne(userPreferencesCollection, inceptiondb.PatchQuery{Filter: JSON{
		"id": preferences.Id, "user_id": auth.User.ID,
	}, Patch: JSON{"locale": locale, "updatedOn": now}}, preferences)
	if err == nil && !matched {
		err = db.Insert(userPreferencesCollection, preferences)
	}
	userPreferenceMu.Unlock()
	if err != nil {
		return nil, HttpError{Status: http.StatusInternalServerError, Title: "Preference Save Failed", Code: "preference_save_failed", Description: "No se pudo guardar el idioma"}
	}
	setLocaleCookie(w, r, locale)
	w.Header().Set("Cache-Control", "no-store")
	return preferences, nil
}

func findUserPreferences(db *inceptiondb.Client, userId string) (*UserPreferences, error) {
	preferences := &UserPreferences{}
	err := db.FindOne(userPreferencesCollection, inceptiondb.FindQuery{Filter: JSON{"id": userPreferenceId(userId), "user_id": userId}}, preferences)
	return preferences, err
}

func userPreferenceId(userId string) string {
	sum := sha256.Sum256([]byte(userId))
	return "preference_" + hex.EncodeToString(sum[:16])
}

func setLocaleCookie(w http.ResponseWriter, r *http.Request, locale i18n.Locale) {
	http.SetCookie(w, &http.Cookie{Name: localeCookieName, Value: string(locale), Path: "/", MaxAge: 365 * 24 * 60 * 60,
		Expires: time.Now().Add(365 * 24 * time.Hour), SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, HttpOnly: true})
}

func localeFromAcceptLanguage(header string) i18n.Locale {
	for _, part := range strings.Split(header, ",") {
		if locale := i18n.Normalize(strings.SplitN(strings.TrimSpace(part), ";", 2)[0]); locale != "" {
			return locale
		}
	}
	return ""
}
