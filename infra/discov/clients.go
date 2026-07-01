package discov

import (
	"fmt"

	"github.com/natuleadan/sdk-api/infra/discov/internal"
)

const timeToLive int64 = 10

// TimeToLive is seconds to live in etcd.
var TimeToLive = timeToLive

func makeEtcdKey(key string, id int64) string {
	return fmt.Sprintf("%s%c%d", key, internal.Delimiter, id)
}
