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

		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = r.RemoteAddr
		}

		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}

		log.Println(r.Method, r.URL.String(), actionName, ip, host, r.Header.Get("User-Agent"))
		next(ctx)
	}
}
