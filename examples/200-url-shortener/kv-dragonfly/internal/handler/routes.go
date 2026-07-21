package handler

import (
	"kv-dragonfly-v2/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func RegisterRoutes(s *runtime.Service, svcCtx *svc.ServiceContext) {
	s.WithRest("createLink", createLink(svcCtx))
	s.WithRest("listLinks", listLinks(svcCtx))
	s.WithRest("getLink", getLink(svcCtx))
	s.WithRest("updateLink", updateLink(svcCtx))
	s.WithRest("deleteLink", deleteLink(svcCtx))
	s.WithRest("expandLink", expandLink(svcCtx))
}
