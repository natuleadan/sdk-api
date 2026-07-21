package svc

import (
	"github.com/natuleadan/sdk-api/infra/stores/redis"
)

type ServiceContext struct {
	Redis *redis.Redis
}

func NewServiceContext(rdb *redis.Redis) *ServiceContext {
	return &ServiceContext{Redis: rdb}
}
