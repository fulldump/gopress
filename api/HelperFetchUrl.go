package api

import (
	"log"
	"net/http"

	"github.com/otiai10/opengraph/v2"

	"gopress/lib/safeurl"
)

func HelperFetchUrl(w http.ResponseWriter, r *http.Request) any {

	link := r.URL.Query().Get("url")

	err := safeurl.AssertSafeUrl(link)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return JSON{"success": 0}
	}

	intent := opengraph.Intent{
		Context:    r.Context(),
		HTTPClient: http.DefaultClient,
		// Strict:      true,
		// TrustedTags: []string{"meta", "title"},
	}
	ogp, err := opengraph.Fetch(link, intent)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return JSON{"success": 0}
	}

	return JSON{
		"success": 1,
		"link":    link,
		"meta": JSON{
			"title":       ogp.Title,
			"description": ogp.Description,
			"image": JSON{
				"url": ogp.Image,
			},
		},
	}
}
