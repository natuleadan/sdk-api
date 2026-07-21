math.randomseed(os.time())

request = function()
    local n = math.random(1000000, 9999999)
    return wrk.format("POST", "/api/nats/rpc", nil, tostring(n))
end
