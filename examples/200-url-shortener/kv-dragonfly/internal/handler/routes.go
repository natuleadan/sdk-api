package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
	"kv-dragonfly-v2/internal/svc"
)

func RegisterRoutes(s *runtime.Service, svcCtx *svc.ServiceContext) {
	s.WithRest("createLink", createLink(svcCtx))
	s.WithRest("listLinks", listLinks(svcCtx))
	s.WithRest("getLink", getLink(svcCtx))
	s.WithRest("updateLink", updateLink(svcCtx))
	s.WithRest("deleteLink", deleteLink(svcCtx))
	s.WithRest("expandLink", expandLink(svcCtx))
}
