math.randomseed(os.time())

request = function()
    local n = math.random(1000000, 9999999)
    return wrk.format("POST", "/api/v1/links",
        {["Content-Type"]="application/json"},
        '{"targetUrl":"https://create-'..n..'.example.com"}'
    )
end
