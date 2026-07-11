request = function()
    local id = math.random(1, 100000)
    wrk.path = "/api/v1/files/sign/bench-" .. id .. ".dat"
    return wrk.format("GET")
end
