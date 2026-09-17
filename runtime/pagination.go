package runtime

import (
	"slices"
	"strconv"
	"strings"

	"github.com/natuleadan/sdk-api/runtime/errcode"
)

// ParseListParams extracts CRUD-compatible pagination from a REST request:
// page/size/sort/cursor plus any extra query args as Filters. Sort is
// validated against sortable when non-empty; pagination selects "offset"
// (default) or "keyset" modes, exactly like CRUD entries.
func ParseListParams(c *RestCtx, pageSize, maxPageSize int, sortable []string, pagination string) (ListParams, error) {
	if pageSize < 1 {
		pageSize = 10
	}
	if maxPageSize < 1 {
		maxPageSize = 100
	}
	if maxPageSize < pageSize {
		maxPageSize = pageSize
	}
	if pagination == "" {
		pagination = "offset"
	}
	size := min(max(queryInt(c, "size", pageSize), pageSize), maxPageSize)
	sort := c.Query("sort", "id")
	if sort == "" {
		sort = "id"
	}
	if len(sortable) > 0 && !slices.Contains(sortable, sort) {
		return ListParams{}, errcode.ErrValidation("sort", "invalid", sort)
	}
	filters := make(map[string]string)
	for key, value := range c.QueryArgs() {
		switch key {
		case "page", "size", "sort", "cursor":
		default:
			filters[key] = value
		}
	}
	return ListParams{
		Page:       max(queryInt(c, "page", 1), 1),
		Size:       size,
		Sort:       sort,
		Filters:    filters,
		Cursor:     c.Query("cursor", ""),
		Pagination: pagination,
	}, nil
}

// queryInt reads an int query param, falling back to def on empty/invalid.
func queryInt(c *RestCtx, key string, def int) int {
	v := strings.TrimSpace(c.Query(key, ""))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
