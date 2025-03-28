package server

// Options holds the server options
type Options struct {
	Auth   AuthOptions
	DryRun bool
}

func (o *Options) Init() {
	o.Auth.Init()
}
