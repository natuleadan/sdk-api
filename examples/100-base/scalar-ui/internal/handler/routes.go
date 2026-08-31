package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
)

func RegisterRoutes(s *runtime.Service) {
	s.WithRest("ping", Ping())
	s.WithRest("echo", Echo())
	s.WithRest("upload", Upload())
	s.WithRest("status", Status())
	s.WithRest("sub", Sub())
	s.WithWS("chat", Chat())
	s.WithSSE("events", Events())
	s.WithAsync("job", Job)
	s.WithCRUD("Product", NewProductCRUD())
	s.RegisterModel("Product", (*Product)(nil))
}
