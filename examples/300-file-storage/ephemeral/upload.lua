wrk.method = "POST"
wrk.body = "benchmark-payload-data-for-upload-testing"
wrk.headers["Content-Type"] = "application/octet-stream"

request = function()
    local id = math.random(100000, 999999)
    wrk.path = "/api/files/upload/bench-" .. id .. ".dat"
    return wrk.format("POST", wrk.path, wrk.headers, wrk.body)
end
