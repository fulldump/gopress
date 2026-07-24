package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/fulldump/box"

	"gopress/glueauth"
	"gopress/templates"
)

func PrettyError(next box.H) box.H {
	return func(ctx context.Context) {
		next(ctx)

		err := box.GetError(ctx)
		if err != nil {

			httpErr, ok := err.(HttpError)
			if errors.Is(err, glueauth.ErrUnauthorized) {
				httpErr = HttpError{
					Status:      http.StatusUnauthorized,
					Title:       "Unauthorized",
					Description: "Debes iniciar sesión para realizar esta acción",
				}
			} else if !ok {
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
				templates.Execute(ctx, "error", w, JSON{
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
