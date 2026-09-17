package redis

import (
	"testing"

	"github.com/natuleadan/sdk-api/infra/stringx"
	"github.com/stretchr/testify/assert"
)

func TestRedisConf(t *testing.T) {
	tests := []struct {
		name string
		RedisConf
		ok bool
	}{
		{
			name: "missing host",
			Host: "",
			Type: NodeType,
			Pass: "",
			ok:   false,
		},
		{
			name: "missing type",
			Host: "localhost:6379",
			Type: "",
			Pass: "",
			ok:   false,
		},
		{
			name: "ok",
			Host: "localhost:6379",
			Type: NodeType,
			Pass: "",
			ok:   true,
		},
		{
			name: "ok",
			Host: "localhost:6379",
			Type: ClusterType,
			Pass: "pwd",
			Tls:  true,
			ok:   true,
		},
		{
			name:       "sentinel",
			Host:       "host1:26379,host2:26379",
			Type:       SentinelType,
			MasterName: "mymaster",
			ok:         true,
		},
	}

	for _, test := range tests {
		t.Run(stringx.RandId(), func(t *testing.T) {
			if test.ok {
				assert.NoError(t, test.Validate())
				assert.NotNil(t, test.NewRedis())
			} else {
				assert.Error(t, test.Validate())
			}
		})
	}
}

func TestRedisKeyConf(t *testing.T) {
	tests := []struct {
		name string
		RedisKeyConf
		ok bool
	}{
		{
			name: "missing host",
			Host: "",
			Type: NodeType,
			Pass: "",
			Key:  "foo",
			ok:   false,
		},
		{
			name: "missing key",
			Host: "localhost:6379",
			Type: NodeType,
			Pass: "",
			Key:  "",
			ok:   false,
		},
		{
			name: "ok",
			Host: "localhost:6379",
			Type: NodeType,
			Pass: "",
			Key:  "foo",
			ok:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.ok {
				assert.NoError(t, test.Validate())
			} else {
				assert.Error(t, test.Validate())
			}
		})
	}
}
