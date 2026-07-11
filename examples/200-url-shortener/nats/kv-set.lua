math.randomseed(os.time())

request = function()
    local n = math.random(1000000, 9999999)
    return wrk.format("PUT", "/api/v1/nats/kv/kvset-" .. n, nil, "bench-value-" .. n)
end
