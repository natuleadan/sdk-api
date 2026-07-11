request = function()
    local id = math.random(1, 200)
    wrk.path = "/api/v1/files/download/hot" .. string.format("%05d", id) .. ".dat"
    return wrk.format("GET")
end
