math.randomseed(os.time())

request = function()
    local n = math.random(100000, 999999)
    local body = "upload-data-" .. n
    return wrk.format("POST", "/api/files/upload",
        {["Content-Type"]="application/octet-stream"},
        body
    )
end
