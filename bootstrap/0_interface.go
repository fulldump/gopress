package bootstrap

type Runner func() (start, stop func() error)
