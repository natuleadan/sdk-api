math.randomseed(os.time())

request = function()
    local id = math.random(1, 200)
    return wrk.format("GET", "/api/links/" .. id)
end
