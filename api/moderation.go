package api

import (
	"context"

	"github.com/fulldump/box"
)

type ContentModerator interface {
	Evaluate(ctx context.Context, content string) (bool, error)
}

type TagGenerator interface {
	GenerateTags(ctx context.Context, content string) ([]string, error)
}

type TranslationGenerator interface {
	TranslateArticle(ctx context.Context, sourceLanguage, targetLanguage, title, contentHTML string) (string, string, error)
}

const contentModeratorKey = "c9c57334-1f5f-4f0d-bc59-79b74e89616d"

func InjectContentModerator(moderator ContentModerator) box.I {
	return func(next box.H) box.H {
		return func(ctx context.Context) {
			if moderator != nil {
				ctx = context.WithValue(ctx, contentModeratorKey, moderator)
			}
			next(ctx)
		}
	}
}

func GetContentModerator(ctx context.Context) ContentModerator {
	if v := ctx.Value(contentModeratorKey); v != nil {
		if moderator, ok := v.(ContentModerator); ok {
			return moderator
		}
	}
	return nil
}

func GetTagGenerator(ctx context.Context) TagGenerator {
	moderator := GetContentModerator(ctx)
	if generator, ok := moderator.(TagGenerator); ok {
		return generator
	}
	return nil
}

func GetTranslationGenerator(ctx context.Context) TranslationGenerator {
	moderator := GetContentModerator(ctx)
	if generator, ok := moderator.(TranslationGenerator); ok {
		return generator
	}
	return nil
}
