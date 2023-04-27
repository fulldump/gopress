package api

import (
	"context"

	"github.com/fulldump/box"

	"gopress/inceptiondb"
)

func InjectInceptionClient(client *inceptiondb.Client) box.I {
	return func(next box.H) box.H {
		return func(ctx context.Context) {
			ctx = SetInceptionClient(ctx, client)
			next(ctx)
		}
	}
}

const InceptionClientKey = "90f39df8-53e8-11ed-97b6-bfcdafd727ba"

func SetInceptionClient(ctx context.Context, client *inceptiondb.Client) context.Context {
	return context.WithValue(ctx, InceptionClientKey, client)
}

func GetInceptionClient(ctx context.Context) *inceptiondb.Client {
	return ctx.Value(InceptionClientKey).(*inceptiondb.Client) // this panics if client is not present
}
