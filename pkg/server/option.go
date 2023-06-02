package server

// Options holds the server options
type Options struct {
	Auth AuthOptions
}

func (o *Options) Init() {
	o.Auth.Init()
}
