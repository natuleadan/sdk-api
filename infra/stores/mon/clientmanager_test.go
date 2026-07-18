package mon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestClientManger_getClient(t *testing.T) {
	c := &mongo.Client{}
	Inject("foo", c)
	cli, err := getClient("foo")
	require.NoError(t, err)
	assert.Equal(t, c, cli)
}
