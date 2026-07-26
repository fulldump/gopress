package api

import "context"

func GetCapabilities(ctx context.Context) JSON {
	return JSON{"automaticTags": GetTagGenerator(ctx) != nil, "automaticTranslations": GetTranslationGenerator(ctx) != nil}
}
