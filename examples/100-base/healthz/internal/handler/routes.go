package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
)

func RegisterRoutes(s *runtime.Service) {
	s.WithRest("ping", Ping())
}
