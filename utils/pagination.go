package utils

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Page    int `json:"page" form:"page"`
	PerPage int `json:"per_page" form:"per_page"`
	Offset  int `json:"offset" form:"offset"`
}

// PaginationResult represents pagination result
type PaginationResult struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
	Offset     int   `json:"offset"`
}

// GetPaginationParams extracts pagination parameters from gin context
func GetPaginationParams(c *gin.Context) PaginationParams {
	page := getIntParam(c, "page", 1)
	perPage := getIntParam(c, "per_page", 10)
	offset := getIntParam(c, "offset", 0)

	// Validate parameters
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 10
	}
	if perPage > 100 {
		perPage = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Calculate offset from page if not provided
	if offset == 0 {
		offset = (page - 1) * perPage
	}

	return PaginationParams{
		Page:    page,
		PerPage: perPage,
		Offset:  offset,
	}
}

// GetPaginationParamsWithDefaults extracts pagination parameters with custom defaults
func GetPaginationParamsWithDefaults(c *gin.Context, defaultPage, defaultPerPage, maxPerPage int) PaginationParams {
	page := getIntParam(c, "page", defaultPage)
	perPage := getIntParam(c, "per_page", defaultPerPage)
	offset := getIntParam(c, "offset", 0)

	// Validate parameters
	if page < 1 {
		page = defaultPage
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	if offset < 0 {
		offset = 0
	}

	// Calculate offset from page if not provided
	if offset == 0 {
		offset = (page - 1) * perPage
	}

	return PaginationParams{
		Page:    page,
		PerPage: perPage,
		Offset:  offset,
	}
}

// CalculatePagination calculates pagination result
func CalculatePagination(page, perPage int, total int64) PaginationResult {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 10
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}

	offset := (page - 1) * perPage

	return PaginationResult{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
		Offset:     offset,
	}
}

// getIntParam gets integer parameter from gin context
func getIntParam(c *gin.Context, key string, defaultValue int) int {
	value := c.Query(key)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return intValue
}

// OffsetPagination represents offset-based pagination
type OffsetPagination struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// GetOffsetPagination gets offset-based pagination parameters
func GetOffsetPagination(c *gin.Context) OffsetPagination {
	offset := getIntParam(c, "offset", 0)
	limit := getIntParam(c, "limit", 10)

	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	return OffsetPagination{
		Offset: offset,
		Limit:  limit,
	}
}

// CursorPagination represents cursor-based pagination
type CursorPagination struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit"`
}

// GetCursorPagination gets cursor-based pagination parameters
func GetCursorPagination(c *gin.Context) CursorPagination {
	after := c.Query("after")
	before := c.Query("before")
	limit := getIntParam(c, "limit", 10)

	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	return CursorPagination{
		After:  after,
		Before: before,
		Limit:  limit,
	}
}

// PageInfo represents page information for cursor-based pagination
type PageInfo struct {
	HasNextPage     bool   `json:"has_next_page"`
	HasPreviousPage bool   `json:"has_previous_page"`
	StartCursor     string `json:"start_cursor,omitempty"`
	EndCursor       string `json:"end_cursor,omitempty"`
}

// CursorPaginationResult represents cursor-based pagination result
type CursorPaginationResult struct {
	PageInfo PageInfo    `json:"page_info"`
	Data     interface{} `json:"data"`
}

// NewCursorPaginationResult creates a new cursor pagination result
func NewCursorPaginationResult(data interface{}, pageInfo PageInfo) CursorPaginationResult {
	return CursorPaginationResult{
		PageInfo: pageInfo,
		Data:     data,
	}
}

// SortOrder represents sort order
type SortOrder string

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)

// SortParams represents sort parameters
type SortParams struct {
	Field string    `json:"field"`
	Order SortOrder `json:"order"`
}

// GetSortParams extracts sort parameters from gin context
func GetSortParams(c *gin.Context, defaultField string, defaultOrder SortOrder) SortParams {
	field := c.Query("sort")
	if field == "" {
		field = defaultField
	}

	order := SortOrder(c.Query("order"))
	if order != SortOrderAsc && order != SortOrderDesc {
		order = defaultOrder
	}

	return SortParams{
		Field: field,
		Order: order,
	}
}

// GetMultipleSortParams extracts multiple sort parameters
func GetMultipleSortParams(c *gin.Context, defaultSorts []SortParams) []SortParams {
	sortFields := c.QueryArray("sort")
	if len(sortFields) == 0 {
		return defaultSorts
	}

	var sorts []SortParams
	for _, field := range sortFields {
		order := SortOrderAsc
		if field[0] == '-' {
			order = SortOrderDesc
			field = field[1:]
		}

		sorts = append(sorts, SortParams{
			Field: field,
			Order: order,
		})
	}

	return sorts
}

// FilterParams represents filter parameters
type FilterParams map[string]interface{}

// GetFilterParams extracts filter parameters from gin context
func GetFilterParams(c *gin.Context, allowedFields []string) FilterParams {
	filters := make(FilterParams)

	for _, field := range allowedFields {
		value := c.Query(field)
		if value != "" {
			filters[field] = value
		}
	}

	return filters
}

// SearchParams represents search parameters
type SearchParams struct {
	Query  string   `json:"query"`
	Fields []string `json:"fields"`
}

// GetSearchParams extracts search parameters from gin context
func GetSearchParams(c *gin.Context, defaultFields []string) SearchParams {
	query := c.Query("q")
	if query == "" {
		query = c.Query("search")
	}

	fields := c.QueryArray("fields")
	if len(fields) == 0 {
		fields = defaultFields
	}

	return SearchParams{
		Query:  query,
		Fields: fields,
	}
}