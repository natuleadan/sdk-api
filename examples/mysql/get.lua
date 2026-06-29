math.randomseed(os.time())

local ids = {}
for i = 1, 100 do
    ids[i] = i
end

request = function()
    return wrk.format("GET", "/api/v1/product/" .. ids[math.random(#ids)])
end
