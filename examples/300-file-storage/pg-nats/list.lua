local cursor = ""

request = function()
    if cursor == "" then
        wrk.path = "/api/v1/products?size=20"
    else
        wrk.path = "/api/v1/products?cursor=" .. cursor .. "&size=20"
    end
    return wrk.format("GET")
end

response = function(status, headers, body)
    if status == 200 then
        local _, _, nextCursor = string.find(body, '"nextCursor":"([^"]+)"')
        if nextCursor and nextCursor ~= "" then
            cursor = nextCursor
        else
            cursor = ""
        end
    else
        cursor = ""
    end
end
