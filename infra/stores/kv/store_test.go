package kv

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/natuleadan/sdk-api/infra/hash"
	"github.com/natuleadan/sdk-api/infra/stores/cache"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/infra/stringx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	s1, _ = miniredis.Run()
	s2, _ = miniredis.Run()
)

func TestRedis_Decr(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Decr("a")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		val, err := client.Decr("a")
		require.NoError(t, err)
		assert.Equal(t, int64(-1), val)
		val, err = client.Decr("a")
		require.NoError(t, err)
		assert.Equal(t, int64(-2), val)
	})
}

func TestRedis_DecrBy(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Incrby("a", 2)
	require.Error(t, err)

	runOnCluster(func(client Store) {
		val, err := client.Decrby("a", 2)
		require.NoError(t, err)
		assert.Equal(t, int64(-2), val)
		val, err = client.Decrby("a", 3)
		require.NoError(t, err)
		assert.Equal(t, int64(-5), val)
	})
}

func TestRedis_Exists(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Exists("foo")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		ok, err := client.Exists("a")
		require.NoError(t, err)
		assert.False(t, ok)
		require.NoError(t, client.Set("a", "b"))
		ok, err = client.Exists("a")
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

func TestRedis_Eval(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Eval(`redis.call("EXISTS", KEYS[1])`, "key1")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		_, err := client.Eval(`redis.call("EXISTS", KEYS[1])`, "notexist")
		assert.Equal(t, redis.Nil, err)
		err = client.Set("key1", "value1")
		require.NoError(t, err)
		_, err = client.Eval(`redis.call("EXISTS", KEYS[1])`, "key1")
		assert.Equal(t, redis.Nil, err)
		val, err := client.Eval(`return redis.call("EXISTS", KEYS[1])`, "key1")
		require.NoError(t, err)
		assert.Equal(t, int64(1), val)
	})
}

func TestRedis_Hgetall(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	err := store.Hset("a", "aa", "aaa")
	require.Error(t, err)
	_, err = store.Hgetall("a")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		assert.NoError(t, client.Hset("a", "aa", "aaa"))
		assert.NoError(t, client.Hset("a", "bb", "bbb"))
		vals, err := client.Hgetall("a")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"aa": "aaa",
			"bb": "bbb",
		}, vals)
	})
}

func TestRedis_Hvals(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Hvals("a")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		assert.NoError(t, client.Hset("a", "aa", "aaa"))
		assert.NoError(t, client.Hset("a", "bb", "bbb"))
		vals, err := client.Hvals("a")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"aaa", "bbb"}, vals)
	})
}

func TestRedis_Hsetnx(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Hsetnx("a", "dd", "ddd")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		assert.NoError(t, client.Hset("a", "aa", "aaa"))
		assert.NoError(t, client.Hset("a", "bb", "bbb"))
		ok, err := client.Hsetnx("a", "bb", "ccc")
		require.NoError(t, err)
		assert.False(t, ok)
		ok, err = client.Hsetnx("a", "dd", "ddd")
		require.NoError(t, err)
		assert.True(t, ok)
		vals, err := client.Hvals("a")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"aaa", "bbb", "ddd"}, vals)
	})
}

func TestRedis_HdelHlen(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Hdel("a", "aa")
	require.Error(t, err)
	_, err = store.Hlen("a")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		assert.NoError(t, client.Hset("a", "aa", "aaa"))
		assert.NoError(t, client.Hset("a", "bb", "bbb"))
		num, err := client.Hlen("a")
		require.NoError(t, err)
		assert.InDelta(t, 2, num, 0.01)
		val, err := client.Hdel("a", "aa")
		require.NoError(t, err)
		assert.True(t, val)
		vals, err := client.Hvals("a")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"bbb"}, vals)
	})
}

func TestRedis_HIncrBy(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Hincrby("key", "field", 3)
	require.Error(t, err)

	runOnCluster(func(client Store) {
		val, err := client.Hincrby("key", "field", 2)
		require.NoError(t, err)
		assert.InDelta(t, 2, val, 0.01)
		val, err = client.Hincrby("key", "field", 3)
		require.NoError(t, err)
		assert.InDelta(t, 5, val, 0.01)
	})
}

func TestRedis_Hkeys(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Hkeys("a")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		assert.NoError(t, client.Hset("a", "aa", "aaa"))
		assert.NoError(t, client.Hset("a", "bb", "bbb"))
		vals, err := client.Hkeys("a")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"aa", "bb"}, vals)
	})
}

func TestRedis_Hmget(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Hmget("a", "aa", "bb")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		assert.NoError(t, client.Hset("a", "aa", "aaa"))
		assert.NoError(t, client.Hset("a", "bb", "bbb"))
		vals, err := client.Hmget("a", "aa", "bb")
		require.NoError(t, err)
		assert.Equal(t, []string{"aaa", "bbb"}, vals)
		vals, err = client.Hmget("a", "aa", "no", "bb")
		require.NoError(t, err)
		assert.Equal(t, []string{"aaa", "", "bbb"}, vals)
	})
}

func TestRedis_Hmset(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	err := store.Hmset("a", map[string]string{
		"aa": "aaa",
	})
	require.Error(t, err)

	runOnCluster(func(client Store) {
		assert.NoError(t, client.Hmset("a", map[string]string{
			"aa": "aaa",
			"bb": "bbb",
		}))
		vals, err := client.Hmget("a", "aa", "bb")
		require.NoError(t, err)
		assert.Equal(t, []string{"aaa", "bbb"}, vals)
	})
}

func TestRedis_Incr(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Incr("a")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		val, err := client.Incr("a")
		require.NoError(t, err)
		assert.Equal(t, int64(1), val)
		val, err = client.Incr("a")
		require.NoError(t, err)
		assert.Equal(t, int64(2), val)
	})
}

func TestRedis_IncrBy(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Incrby("a", 2)
	require.Error(t, err)

	runOnCluster(func(client Store) {
		val, err := client.Incrby("a", 2)
		require.NoError(t, err)
		assert.Equal(t, int64(2), val)
		val, err = client.Incrby("a", 3)
		require.NoError(t, err)
		assert.Equal(t, int64(5), val)
	})
}

func TestRedis_List(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Lpush("key", "value1", "value2")
	require.Error(t, err)
	_, err = store.Rpush("key", "value3", "value4")
	require.Error(t, err)
	_, err = store.Llen("key")
	require.Error(t, err)
	_, err = store.Lrange("key", 0, 10)
	require.Error(t, err)
	_, err = store.Lpop("key")
	require.Error(t, err)
	_, err = store.Lrem("key", 0, "val")
	require.Error(t, err)
	_, err = store.Lindex("key", 0)
	require.Error(t, err)

	runOnCluster(func(client Store) {
		val, err := client.Lpush("key", "value1", "value2")
		require.NoError(t, err)
		assert.InDelta(t, 2, val, 0.01)
		val, err = client.Rpush("key", "value3", "value4")
		require.NoError(t, err)
		assert.InDelta(t, 4, val, 0.01)
		val, err = client.Llen("key")
		require.NoError(t, err)
		assert.InDelta(t, 4, val, 0.01)
		value, err := client.Lindex("key", 0)
		require.NoError(t, err)
		assert.Equal(t, "value2", value)
		vals, err := client.Lrange("key", 0, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"value2", "value1", "value3", "value4"}, vals)
		v, err := client.Lpop("key")
		require.NoError(t, err)
		assert.Equal(t, "value2", v)
		val, err = client.Lpush("key", "value1", "value2")
		require.NoError(t, err)
		assert.InDelta(t, 5, val, 0.01)
		val, err = client.Rpush("key", "value3", "value3")
		require.NoError(t, err)
		assert.InDelta(t, 7, val, 0.01)
		n, err := client.Lrem("key", 2, "value1")
		require.NoError(t, err)
		assert.InDelta(t, 2, n, 0.01)
		vals, err = client.Lrange("key", 0, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"value2", "value3", "value4", "value3", "value3"}, vals)
		n, err = client.Lrem("key", -2, "value3")
		require.NoError(t, err)
		assert.InDelta(t, 2, n, 0.01)
		vals, err = client.Lrange("key", 0, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"value2", "value3", "value4"}, vals)
	})
}

func TestRedis_Persist(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Persist("key")
	require.Error(t, err)
	err = store.Expire("key", 5)
	require.Error(t, err)
	err = store.Expireat("key", time.Now().Unix()+5)
	require.Error(t, err)

	runOnCluster(func(client Store) {
		ok, err := client.Persist("key")
		require.NoError(t, err)
		assert.False(t, ok)
		err = client.Set("key", "value")
		require.NoError(t, err)
		ok, err = client.Persist("key")
		require.NoError(t, err)
		assert.False(t, ok)
		err = client.Expire("key", 5)
		require.NoError(t, err)
		ok, err = client.Persist("key")
		require.NoError(t, err)
		assert.True(t, ok)
		err = client.Expireat("key", time.Now().Unix()+5)
		require.NoError(t, err)
		ok, err = client.Persist("key")
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

func TestRedis_Sscan(t *testing.T) {
	key := "list"
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Sadd(key, nil)
	require.Error(t, err)
	_, _, err = store.Sscan(key, 0, "", 100)
	require.Error(t, err)
	_, err = store.Del(key)
	require.Error(t, err)

	runOnCluster(func(client Store) {
		var list []string
		for i := range 1550 {
			list = append(list, stringx.Randn(i))
		}
		lens, err := client.Sadd(key, list)
		require.NoError(t, err)
		assert.InDelta(t, 1550, lens, 0.01)

		var cursor uint64 = 0
		sum := 0
		for {
			keys, next, err := client.Sscan(key, cursor, "", 100)
			require.NoError(t, err)
			sum += len(keys)
			if next == 0 {
				break
			}
			cursor = next
		}

		assert.InDelta(t, 1550, sum, 0.01)
		_, err = client.Del(key)
		require.NoError(t, err)
	})
}

func TestRedis_Set(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Scard("key")
	require.Error(t, err)
	_, err = store.Sismember("key", 2)
	require.Error(t, err)
	_, err = store.Srem("key", 3, 4)
	require.Error(t, err)
	_, err = store.Smembers("key")
	require.Error(t, err)
	_, err = store.Srandmember("key", 1)
	require.Error(t, err)
	_, err = store.Spop("key")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		num, err := client.Sadd("key", 1, 2, 3, 4)
		require.NoError(t, err)
		assert.InDelta(t, 4, num, 0.01)
		val, err := client.Scard("key")
		require.NoError(t, err)
		assert.Equal(t, int64(4), val)
		ok, err := client.Sismember("key", 2)
		require.NoError(t, err)
		assert.True(t, ok)
		num, err = client.Srem("key", 3, 4)
		require.NoError(t, err)
		assert.InDelta(t, 2, num, 0.01)
		vals, err := client.Smembers("key")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"1", "2"}, vals)
		members, err := client.Srandmember("key", 1)
		require.NoError(t, err)
		assert.Len(t, members, 1)
		assert.Contains(t, []string{"1", "2"}, members[0])
		member, err := client.Spop("key")
		require.NoError(t, err)
		assert.Contains(t, []string{"1", "2"}, member)
		vals, err = client.Smembers("key")
		require.NoError(t, err)
		assert.NotContains(t, vals, member)
		num, err = client.Sadd("key1", 1, 2, 3, 4)
		require.NoError(t, err)
		assert.InDelta(t, 4, num, 0.01)
		num, err = client.Sadd("key2", 2, 3, 4, 5)
		require.NoError(t, err)
		assert.InDelta(t, 4, num, 0.01)
	})
}

func TestRedis_SetGetDel(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	err := store.Set("hello", "world")
	require.Error(t, err)
	_, err = store.Get("hello")
	require.Error(t, err)
	_, err = store.Del("hello")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		err := client.Set("hello", "world")
		require.NoError(t, err)
		val, err := client.Get("hello")
		require.NoError(t, err)
		assert.Equal(t, "world", val)
		ret, err := client.Del("hello")
		require.NoError(t, err)
		assert.InDelta(t, 1, ret, 0.01)
	})
}

func TestRedis_SetExNx(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	err := store.Setex("hello", "world", 5)
	require.Error(t, err)
	_, err = store.Setnx("newhello", "newworld")
	require.Error(t, err)
	_, err = store.Ttl("hello")
	require.Error(t, err)
	_, err = store.SetnxEx("newhello", "newworld", 5)
	require.Error(t, err)

	runOnCluster(func(client Store) {
		err := client.Setex("hello", "world", 5)
		require.NoError(t, err)
		ok, err := client.Setnx("hello", "newworld")
		require.NoError(t, err)
		assert.False(t, ok)
		ok, err = client.Setnx("newhello", "newworld")
		require.NoError(t, err)
		assert.True(t, ok)
		val, err := client.Get("hello")
		require.NoError(t, err)
		assert.Equal(t, "world", val)
		val, err = client.Get("newhello")
		require.NoError(t, err)
		assert.Equal(t, "newworld", val)
		ttl, err := client.Ttl("hello")
		require.NoError(t, err)
		assert.Positive(t, ttl)
		ok, err = client.SetnxEx("newhello", "newworld", 5)
		require.NoError(t, err)
		assert.False(t, ok)
		num, err := client.Del("newhello")
		require.NoError(t, err)
		assert.InDelta(t, 1, num, 0.01)
		ok, err = client.SetnxEx("newhello", "newworld", 5)
		require.NoError(t, err)
		assert.True(t, ok)
		val, err = client.Get("newhello")
		require.NoError(t, err)
		assert.Equal(t, "newworld", val)
	})
}

func TestRedis_Getset(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.GetSet("hello", "world")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		val, err := client.GetSet("hello", "world")
		require.NoError(t, err)
		assert.Empty(t, val)
		val, err = client.Get("hello")
		require.NoError(t, err)
		assert.Equal(t, "world", val)
		val, err = client.GetSet("hello", "newworld")
		require.NoError(t, err)
		assert.Equal(t, "world", val)
		val, err = client.Get("hello")
		require.NoError(t, err)
		assert.Equal(t, "newworld", val)
		_, err = client.Del("hello")
		require.NoError(t, err)
	})
}

func TestRedis_SetGetDelHashField(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	err := store.Hset("key", "field", "value")
	require.Error(t, err)
	_, err = store.Hget("key", "field")
	require.Error(t, err)
	_, err = store.Hexists("key", "field")
	require.Error(t, err)
	_, err = store.Hdel("key", "field")
	require.Error(t, err)

	runOnCluster(func(client Store) {
		err := client.Hset("key", "field", "value")
		require.NoError(t, err)
		val, err := client.Hget("key", "field")
		require.NoError(t, err)
		assert.Equal(t, "value", val)
		ok, err := client.Hexists("key", "field")
		require.NoError(t, err)
		assert.True(t, ok)
		ret, err := client.Hdel("key", "field")
		require.NoError(t, err)
		assert.True(t, ret)
		ok, err = client.Hexists("key", "field")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestRedis_SortedSet(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Zadd("key", 1, "value1")
	require.Error(t, err)
	_, err = store.Zscore("key", "value1")
	require.Error(t, err)
	_, err = store.Zcount("key", 6, 7)
	require.Error(t, err)
	_, err = store.Zincrby("key", 3, "value1")
	require.Error(t, err)
	_, err = store.Zrank("key", "value2")
	require.Error(t, err)
	_, err = store.Zrem("key", "value2", "value3")
	require.Error(t, err)
	_, err = store.Zremrangebyscore("key", 6, 7)
	require.Error(t, err)
	_, err = store.Zremrangebyrank("key", 1, 2)
	require.Error(t, err)
	_, err = store.Zcard("key")
	require.Error(t, err)
	_, err = store.Zrange("key", 0, -1)
	require.Error(t, err)
	_, err = store.Zrevrange("key", 0, -1)
	require.Error(t, err)
	_, err = store.ZrangeWithScores("key", 0, -1)
	require.Error(t, err)
	_, err = store.ZrangebyscoreWithScores("key", 5, 8)
	require.Error(t, err)
	_, err = store.ZrangebyscoreWithScoresAndLimit("key", 5, 8, 1, 1)
	require.Error(t, err)
	_, err = store.ZrevrangebyscoreWithScores("key", 5, 8)
	require.Error(t, err)
	_, err = store.ZrevrangebyscoreWithScoresAndLimit("key", 5, 8, 1, 1)
	require.Error(t, err)
	_, err = store.Zrevrank("key", "value")
	require.Error(t, err)
	_, err = store.Zadds("key", redis.Pair{
		Key:   "value2",
		Score: 6,
	}, redis.Pair{
		Key:   "value3",
		Score: 7,
	})
	require.Error(t, err)

	runOnCluster(func(client Store) {
		ok, err := client.ZaddFloat("key", 1, "value1")
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = client.Zadd("key", 2, "value1")
		require.NoError(t, err)
		assert.False(t, ok)
		val, err := client.Zscore("key", "value1")
		require.NoError(t, err)
		assert.Equal(t, int64(2), val)
		val, err = client.Zincrby("key", 3, "value1")
		require.NoError(t, err)
		assert.Equal(t, int64(5), val)
		val, err = client.Zscore("key", "value1")
		require.NoError(t, err)
		assert.Equal(t, int64(5), val)
		ok, err = client.Zadd("key", 6, "value2")
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = client.Zadd("key", 7, "value3")
		require.NoError(t, err)
		assert.True(t, ok)
		rank, err := client.Zrank("key", "value2")
		require.NoError(t, err)
		assert.Equal(t, int64(1), rank)
		_, err = client.Zrank("key", "value4")
		assert.Equal(t, redis.Nil, err)
		num, err := client.Zrem("key", "value2", "value3")
		require.NoError(t, err)
		assert.InDelta(t, 2, num, 0.01)
		ok, err = client.Zadd("key", 6, "value2")
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = client.Zadd("key", 7, "value3")
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = client.Zadd("key", 8, "value4")
		require.NoError(t, err)
		assert.True(t, ok)
		num, err = client.Zremrangebyscore("key", 6, 7)
		require.NoError(t, err)
		assert.InDelta(t, 2, num, 0.01)
		ok, err = client.Zadd("key", 6, "value2")
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = client.Zadd("key", 7, "value3")
		require.NoError(t, err)
		assert.True(t, ok)
		num, err = client.Zcount("key", 6, 7)
		require.NoError(t, err)
		assert.InDelta(t, 2, num, 0.01)
		num, err = client.Zremrangebyrank("key", 1, 2)
		require.NoError(t, err)
		assert.InDelta(t, 2, num, 0.01)
		card, err := client.Zcard("key")
		require.NoError(t, err)
		assert.InDelta(t, 2, card, 0.01)
		vals, err := client.Zrange("key", 0, -1)
		require.NoError(t, err)
		assert.Equal(t, []string{"value1", "value4"}, vals)
		vals, err = client.Zrevrange("key", 0, -1)
		require.NoError(t, err)
		assert.Equal(t, []string{"value4", "value1"}, vals)
		pairs, err := client.ZrangeWithScores("key", 0, -1)
		require.NoError(t, err)
		assert.Equal(t, []redis.Pair{
			{
				Key:   "value1",
				Score: 5,
			},
			{
				Key:   "value4",
				Score: 8,
			},
		}, pairs)
		pairs, err = client.ZrangebyscoreWithScores("key", 5, 8)
		require.NoError(t, err)
		assert.Equal(t, []redis.Pair{
			{
				Key:   "value1",
				Score: 5,
			},
			{
				Key:   "value4",
				Score: 8,
			},
		}, pairs)
		pairs, err = client.ZrangebyscoreWithScoresAndLimit("key", 5, 8, 1, 1)
		require.NoError(t, err)
		assert.Equal(t, []redis.Pair{
			{
				Key:   "value4",
				Score: 8,
			},
		}, pairs)
		pairs, err = client.ZrevrangebyscoreWithScores("key", 5, 8)
		require.NoError(t, err)
		assert.Equal(t, []redis.Pair{
			{
				Key:   "value4",
				Score: 8,
			},
			{
				Key:   "value1",
				Score: 5,
			},
		}, pairs)
		pairs, err = client.ZrevrangebyscoreWithScoresAndLimit("key", 5, 8, 1, 1)
		require.NoError(t, err)
		assert.Equal(t, []redis.Pair{
			{
				Key:   "value1",
				Score: 5,
			},
		}, pairs)
		rank, err = client.Zrevrank("key", "value1")
		require.NoError(t, err)
		assert.Equal(t, int64(1), rank)
		val, err = client.Zadds("key", redis.Pair{
			Key:   "value2",
			Score: 6,
		}, redis.Pair{
			Key:   "value3",
			Score: 7,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), val)
	})
}

func TestRedis_HyperLogLog(t *testing.T) {
	store := clusterStore{dispatcher: hash.NewConsistentHash()}
	_, err := store.Pfadd("key")
	require.Error(t, err)
	_, err = store.Pfcount("key")
	require.Error(t, err)

	runOnCluster(func(cluster Store) {
		ok, err := cluster.Pfadd("key", "value")
		require.NoError(t, err)
		assert.True(t, ok)
		val, err := cluster.Pfcount("key")
		require.NoError(t, err)
		assert.Equal(t, int64(1), val)
	})
}

func runOnCluster(fn func(cluster Store)) {
	s1.FlushAll()
	s2.FlushAll()

	store := NewStore([]cache.NodeConf{
		{
			RedisConf: redis.RedisConf{
				Host: s1.Addr(),
				Type: redis.NodeType,
			},
			Weight: 100,
		},
		{
			RedisConf: redis.RedisConf{
				Host: s2.Addr(),
				Type: redis.NodeType,
			},
			Weight: 100,
		},
	})

	fn(store)
}
