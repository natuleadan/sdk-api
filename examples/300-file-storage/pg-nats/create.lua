wrk.method = "POST"
wrk.headers["Content-Type"] = "application/json"

request = function()
    local id = math.random(1, 100000)
    wrk.body = '{"name":"product-' .. id .. '","price":' .. (id % 100) .. '.99}'
    wrk.path = "/api/v1/products"
    return wrk.format("POST", wrk.path, wrk.headers, wrk.body)
end
