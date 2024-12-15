package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/fulldump/box"

	"gopress/templates"
)

func PrettyError(next box.H) box.H {
	return func(ctx context.Context) {
		next(ctx)

		err := box.GetError(ctx)
		if err != nil {

			httpErr, ok := err.(HttpError)
			if !ok {
				httpErr = HttpError{
					Status:      http.StatusInternalServerError,
					Title:       "Unexpected error",
					Description: err.Error(),
				}
			}

			w := box.GetResponse(ctx)
			w.Header().Set("X-Robots-Tag", "noindex,nofollow")
			w.WriteHeader(httpErr.Status)

			r := box.GetRequest(ctx)

			if strings.Contains(r.Header.Get("Accept"), "text/html") {
				templates.GetByName(ctx, "error").ExecuteTemplate(w, "", JSON{
					"error": httpErr,
				})
			} else {
				json.NewEncoder(w).Encode(JSON{
					"error": httpErr,
				})
			}
		}
	}
}
