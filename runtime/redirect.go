package runtime

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// Redirect provides a fluent API for HTTP redirects with flash message support.
// It wraps Fiber's redirect functionality and adds SDK-level conveniences.
//
// Usage:
//
//	// Simple redirect
//	return c.Redirect().To("/login")
//
//	// Redirect with status
//	return c.Redirect().Status(fiber.StatusMovedPermanently).To("/new-path")
//
//	// Redirect to named route with params
//	return c.Redirect().Route("user", runtime.RedirectConfig{
//	    Params: runtime.Map{"id": "123"},
//	})
//
//	// Redirect back to referer (falls back to "/" if missing)
//	return c.Redirect().Back("/")
//
//	// Flash message + redirect
//	return c.Redirect().With("status", "Logged in").To("/dashboard")
//
//	// Flash form input + redirect
//	return c.Redirect().WithInput().To("/form")
type Redirect struct {
	ctx fiber.Ctx
}

// newRedirect creates a Redirect from a fiber.Ctx.
func newRedirect(c fiber.Ctx) *Redirect {
	return &Redirect{ctx: c}
}

// To redirects to the given location URL.
// Default status is 303 See Other unless Status() was called first.
func (r *Redirect) To(location string) error {
	return r.ctx.Redirect().To(location)
}

// Status sets the HTTP status code for the redirect.
// It is chainable: c.Redirect().Status(301).To("/path")
func (r *Redirect) Status(code int) *Redirect {
	r.ctx.Redirect().Status(code)
	return r
}

// RedirectConfig configures a redirect to a named route.
type RedirectConfig struct {
	// Params are the route parameters (e.g. {"id": "123"} for /user/:id).
	Params Map
	// Queries are query string parameters merged into the redirect URL.
	Queries map[string]string
}

// Route redirects to a named route with optional parameters and queries.
// The route must have been registered with .Name() on the Fiber app.
//
// Example:
//
//	app.Get("/user/:id", handler).Name("user")
//	// ...
//	return c.Redirect().Route("user", RedirectConfig{
//	    Params:  Map{"id": "42"},
//	    Queries: map[string]string{"tab": "profile"},
//	})
func (r *Redirect) Route(name string, config ...RedirectConfig) error {
	var cfg RedirectConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	fc := fiber.RedirectConfig{}
	if cfg.Params != nil {
		fc.Params = fiber.Map(cfg.Params)
	}
	if cfg.Queries != nil {
		fc.Queries = cfg.Queries
	}
	return r.ctx.Redirect().Route(name, fc)
}

// Back redirects to the Referer header if it is same-origin, otherwise
// falls back to the provided fallback URL. If no fallback is given and
// the referer is missing or cross-origin, it redirects to "/".
//
// Same-origin check normalizes the referer and compares against the
// request host. Backslashes are folded, ASCII tab/CR/LF are dropped,
// and leading slash runs are collapsed (matching browser behavior).
func (r *Redirect) Back(fallback ...string) error {
	return r.ctx.Redirect().Back(fallback...)
}

// With stores a flash message that will be available on the next request.
// Flash messages are stored in a cookie and read via Messages() or Message(key).
//
// Example:
//
//	return c.Redirect().With("error", "Invalid credentials").To("/login")
//	// On next request:
//	msg := c.Redirect().Message("error") // "Invalid credentials"
func (r *Redirect) With(key, value string) *Redirect {
	r.ctx.Redirect().With(key, value)
	return r
}

// WithInput stores the current request's form/query data as flash input.
// The data is stored in a cookie and retrievable via OldInputs() or OldInput(key).
// Captures form, multipart, or query data depending on Content-Type.
//
// Caution: WithInput copies the whole submitted body into a cookie.
// Sensitive fields (passwords) will be visible. Prefer With() for specific fields.
func (r *Redirect) WithInput() *Redirect {
	r.ctx.Redirect().WithInput()
	return r
}

// Messages retrieves all flash messages stored by With() on the previous request.
func (r *Redirect) Messages() []FlashMessage {
	fmsgs := r.ctx.Redirect().Messages()
	msgs := make([]FlashMessage, len(fmsgs))
	for i, m := range fmsgs {
		msgs[i] = FlashMessage{Key: m.Key, Value: m.Value, Level: m.Level}
	}
	return msgs
}

// Message retrieves a single flash message by key.
// Returns an empty FlashMessage if the key does not exist.
func (r *Redirect) Message(key string) FlashMessage {
	m := r.ctx.Redirect().Message(key)
	return FlashMessage{Key: m.Key, Value: m.Value, Level: m.Level}
}

// OldInputs retrieves all input data stored by WithInput() on the previous request.
func (r *Redirect) OldInputs() []OldInputData {
	finputs := r.ctx.Redirect().OldInputs()
	inputs := make([]OldInputData, len(finputs))
	for i, in := range finputs {
		inputs[i] = OldInputData{Key: in.Key, Value: in.Value}
	}
	return inputs
}

// OldInput retrieves a single old input by key.
// Returns an empty OldInputData if the key does not exist.
func (r *Redirect) OldInput(key string) OldInputData {
	in := r.ctx.Redirect().OldInput(key)
	return OldInputData{Key: in.Key, Value: in.Value}
}

// FlashMessage holds a key-value flash message with an optional severity level.
type FlashMessage struct {
	Key   string
	Value string
	Level uint8
}

// OldInputData holds a key-value pair from a previous form submission.
type OldInputData struct {
	Key   string
	Value string
}

// IsSameOrigin checks whether the given referer URL is same-origin with
// the request. This is the same check used by Back() internally.
// Exported for testing and custom redirect logic.
func IsSameOrigin(referer string, requestHost string) bool {
	if referer == "" || requestHost == "" {
		return false
	}
	u, err := url.Parse(referer)
	if err != nil {
		return false
	}
	refHost := u.Hostname()
	if refHost == "" {
		return false
	}
	refPort := u.Port()
	reqHost, reqPort := splitHostPort(requestHost)
	if !strings.EqualFold(refHost, reqHost) {
		return false
	}
	// Normalize default ports
	refPort = normalizePort(refPort, u.Scheme)
	reqPort = normalizePort(reqPort, "")
	return refPort == reqPort
}

func splitHostPort(host string) (string, string) {
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i], host[i+1:]
		}
	}
	return host, ""
}

func normalizePort(port, scheme string) string {
	if port == "" {
		return ""
	}
	if port == "80" && (scheme == "" || scheme == "http") {
		return ""
	}
	if port == "443" && scheme == "https" {
		return ""
	}
	return port
}
