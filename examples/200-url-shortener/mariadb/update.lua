math.randomseed(os.time())

request = function()
    local id = math.random(1, 200)
    return wrk.format("PUT", "/api/v1/links/" .. id,
        {["Content-Type"]="application/json"},
        '{"targetUrl":"https://updated-'..id..'.example.com"}'
    )
end
