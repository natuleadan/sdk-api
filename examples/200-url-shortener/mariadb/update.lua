math.randomseed(os.time())

request = function()
    local id = math.random(1, 2000)
    return wrk.format("PATCH", "/api/links/" .. id,
        {["Content-Type"]="application/json"},
        '{"targetUrl":"https://updated-'..id..'.example.com"}'
    )
end
