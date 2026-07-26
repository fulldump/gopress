package api

import "strings"

const (
	articleTagsAutomatic = "automatic"
	articleTagsManual    = "manual"
	maxArticleTags       = 12
	maxArticleTagLength  = 60
)

func normalizeArticleTags(tags []string) ([]string, error) {
	result := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, value := range tags {
		tag := strings.TrimSpace(value)
		if tag == "" {
			continue
		}
		if len([]rune(tag)) > maxArticleTagLength {
			return nil, invalidArticle("cada tag puede tener un máximo de 60 caracteres")
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, tag)
		if len(result) > maxArticleTags {
			return nil, invalidArticle("un artículo puede tener un máximo de 12 tags")
		}
	}
	return result, nil
}

func articleTagsAreAutomatic(mode string) bool {
	return mode == "" || mode == articleTagsAutomatic
}
