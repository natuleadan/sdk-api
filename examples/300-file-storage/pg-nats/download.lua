math.randomseed(os.time())

local codes = {
    "hot00001","hot00002","hot00003","hot00004","hot00005",
    "hot00006","hot00007","hot00008","hot00009","hot00010",
    "hot00011","hot00012","hot00013","hot00014","hot00015",
    "hot00016","hot00017","hot00018","hot00019","hot00020",
    "hot00021","hot00022","hot00023","hot00024","hot00025",
    "hot00026","hot00027","hot00028","hot00029","hot00030",
    "hot00031","hot00032","hot00033","hot00034","hot00035",
    "hot00036","hot00037","hot00038","hot00039","hot00040",
    "hot00041","hot00042","hot00043","hot00044","hot00045",
    "hot00046","hot00047","hot00048","hot00049","hot00050",
    "hot00051","hot00052","hot00053","hot00054","hot00055",
    "hot00056","hot00057","hot00058","hot00059","hot00060",
    "hot00061","hot00062","hot00063","hot00064","hot00065",
    "hot00066","hot00067","hot00068","hot00069","hot00070",
    "hot00071","hot00072","hot00073","hot00074","hot00075",
    "hot00076","hot00077","hot00078","hot00079","hot00080",
    "hot00081","hot00082","hot00083","hot00084","hot00085",
    "hot00086","hot00087","hot00088","hot00089","hot00090",
    "hot00091","hot00092","hot00093","hot00094","hot00095",
    "hot00096","hot00097","hot00098","hot00099","hot00100",
}

request = function()
    return wrk.format("GET", "/api/files/download/" .. codes[math.random(#codes)])
end
