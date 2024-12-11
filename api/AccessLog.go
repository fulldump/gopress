package api

import (
	"context"
	"log"

	"github.com/fulldump/box"
)

func AccessLog(next box.H) box.H {
	return func(ctx context.Context) {
		r := box.GetRequest(ctx)

		action := box.GetBoxContext(ctx).Action
		actionName := ""
		if action != nil {
			actionName = action.Name
		}

		log.Println(r.Method, r.URL.String(), actionName)
		next(ctx)
	}
}
