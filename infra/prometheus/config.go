package prometheus

// A Config is a prometheus config.
type Config struct {
	Host string `config:",optional"`
	Port int    `config:",default=9101"`
	Path string `config:",default=/metrics"`
}
