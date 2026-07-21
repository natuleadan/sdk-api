request = function()
    local id = math.random(1, 100000)
    wrk.path = "/api/files/sign/bench-" .. id .. ".dat"
    return wrk.format("GET")
end
