package server

// Options holds the server options
type Options struct {
	Auth JWTOptions
}

func (o *Options) Init() {
	o.Auth.Init()
}
