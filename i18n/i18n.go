package i18n

import (
	"context"
	"strings"
)

type Locale string

const (
	Spanish Locale = "es"
	English Locale = "en"
	Chinese Locale = "zh"
)

type contextKey struct{}

func Normalize(value string) Locale {
	value = strings.ToLower(strings.TrimSpace(value))
	if index := strings.IndexAny(value, "-_"); index >= 0 {
		value = value[:index]
	}
	if value == string(English) {
		return English
	}
	if value == string(Spanish) {
		return Spanish
	}
	if value == string(Chinese) {
		return Chinese
	}
	return ""
}

func SetLocale(ctx context.Context, locale Locale) context.Context {
	if locale == "" {
		locale = Spanish
	}
	return context.WithValue(ctx, contextKey{}, locale)
}

func GetLocale(ctx context.Context) Locale {
	if locale, ok := ctx.Value(contextKey{}).(Locale); ok && locale != "" {
		return locale
	}
	return Spanish
}

func Translate(locale Locale, key string) string {
	if messages := catalogs[locale]; messages != nil {
		if value := messages[key]; value != "" {
			return value
		}
	}
	if value := catalogs[Spanish][key]; value != "" {
		return value
	}
	return key
}

func ClientMessages(locale Locale) map[string]string {
	result := map[string]string{}
	for key, value := range catalogs[Spanish] {
		result[key] = value
	}
	for key, value := range catalogs[locale] {
		result[key] = value
	}
	return result
}

var catalogs = map[Locale]map[string]string{
	Spanish: spanishMessages,
	English: englishMessages,
	Chinese: chineseMessages,
}
