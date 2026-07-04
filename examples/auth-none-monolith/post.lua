request = function()
    return wrk.format("POST", "/api/v1/posts", {["Content-Type"] = "application/json"}, '{"title":"wrk","content":"benchmark"}')
end
