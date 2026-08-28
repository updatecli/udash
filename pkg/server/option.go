package server

// Options holds the server options
type Options struct {
	Auth AuthOptions
}

// Init fills in the defaults and reports what it cannot make sense of.
func (o *Options) Init() error {
	return o.Auth.Init()
}
