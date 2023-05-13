package filestorage

import (
	"context"
	"github.com/fulldump/box"
)

const key = "b0d2e5fe-a45c-11ea-9b4e-e7f27822e005"

func Interceptor(f Filestorager) box.I {
	return func(next box.H) box.H {
		return func(ctx context.Context) {

			next(Set(ctx, f))

		}
	}
}

func Set(ctx context.Context, config Filestorager) context.Context {
	return context.WithValue(ctx, key, config)
}

func Get(ctx context.Context) Filestorager {
	value := ctx.Value(key)
	if value == nil {
		panic("Filestorager should be in context!!!!")
	}

	return value.(Filestorager) // TODO: handle casting error
}
