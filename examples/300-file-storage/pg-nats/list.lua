request = function()
    wrk.path = "/api/v1/products"
    return wrk.format("GET")
end
