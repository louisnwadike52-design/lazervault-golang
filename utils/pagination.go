package utils

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PaginationParams holds pagination parameters
type PaginationParams struct {
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// PaginationMeta holds pagination metadata
type PaginationMeta struct {
	CurrentPage  int   `json:"current_page"`
	PageSize     int   `json:"page_size"`
	TotalPages   int   `json:"total_pages"`
	TotalRecords int64 `json:"total_records"`
	HasNext      bool  `json:"has_next"`
	HasPrevious  bool  `json:"has_previous"`
}

// PaginatedResponse wraps paginated data with metadata
type PaginatedResponse struct {
	Data       interface{}     `json:"data"`
	Pagination *PaginationMeta `json:"pagination"`
}

// DefaultPaginationParams returns default pagination parameters
func DefaultPaginationParams() *PaginationParams {
	return &PaginationParams{
		Page:      1,
		PageSize:  20,
		SortBy:    "created_at",
		SortOrder: "desc",
	}
}

// GetPaginationParams extracts pagination parameters from Gin context
func GetPaginationParams(c *gin.Context) *PaginationParams {
	params := DefaultPaginationParams()

	// Get page number
	if pageStr := c.Query("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			params.Page = page
		}
	}

	// Get page size
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil && pageSize > 0 {
			params.PageSize = pageSize
		}
	}

	// Limit page size to prevent abuse
	if params.PageSize > 100 {
		params.PageSize = 100
	}

	// Get sort parameters
	if sortBy := c.Query("sort_by"); sortBy != "" {
		params.SortBy = sortBy
	}

	if sortOrder := c.Query("sort_order"); sortOrder == "asc" || sortOrder == "desc" {
		params.SortOrder = sortOrder
	}

	return params
}

// ApplyPagination applies pagination to a GORM query
func ApplyPagination(db *gorm.DB, params *PaginationParams) *gorm.DB {
	offset := (params.Page - 1) * params.PageSize

	query := db.Offset(offset).Limit(params.PageSize)

	// Apply sorting
	if params.SortBy != "" {
		orderClause := fmt.Sprintf("%s %s", params.SortBy, params.SortOrder)
		query = query.Order(orderClause)
	}

	return query
}

// CalculatePaginationMeta calculates pagination metadata
func CalculatePaginationMeta(params *PaginationParams, totalRecords int64) *PaginationMeta {
	totalPages := int(math.Ceil(float64(totalRecords) / float64(params.PageSize)))

	if totalPages < 1 {
		totalPages = 1
	}

	return &PaginationMeta{
		CurrentPage:  params.Page,
		PageSize:     params.PageSize,
		TotalPages:   totalPages,
		TotalRecords: totalRecords,
		HasNext:      params.Page < totalPages,
		HasPrevious:  params.Page > 1,
	}
}

// Paginate is a helper function that handles the complete pagination flow
func Paginate(db *gorm.DB, params *PaginationParams, dest interface{}) (*PaginatedResponse, error) {
	// Count total records
	var totalRecords int64
	if err := db.Count(&totalRecords).Error; err != nil {
		return nil, fmt.Errorf("failed to count records: %w", err)
	}

	// Apply pagination and fetch records
	if err := ApplyPagination(db, params).Find(dest).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch paginated records: %w", err)
	}

	// Calculate metadata
	meta := CalculatePaginationMeta(params, totalRecords)

	return &PaginatedResponse{
		Data:       dest,
		Pagination: meta,
	}, nil
}

// CursorPaginationParams holds cursor-based pagination parameters
type CursorPaginationParams struct {
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"` // Usually an encoded timestamp or ID
	SortBy string `json:"sort_by"`
}

// DefaultCursorPaginationParams returns default cursor pagination parameters
func DefaultCursorPaginationParams() *CursorPaginationParams {
	return &CursorPaginationParams{
		Limit:  20,
		Cursor: "",
		SortBy: "created_at",
	}
}

// GetCursorPaginationParams extracts cursor pagination parameters from Gin context
func GetCursorPaginationParams(c *gin.Context) *CursorPaginationParams {
	params := DefaultCursorPaginationParams()

	// Get limit
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			params.Limit = limit
		}
	}

	// Limit to prevent abuse
	if params.Limit > 100 {
		params.Limit = 100
	}

	// Get cursor
	if cursor := c.Query("cursor"); cursor != "" {
		params.Cursor = cursor
	}

	// Get sort field
	if sortBy := c.Query("sort_by"); sortBy != "" {
		params.SortBy = sortBy
	}

	return params
}

// CursorPaginationMeta holds cursor pagination metadata
type CursorPaginationMeta struct {
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Limit      int    `json:"limit"`
}

// CursorPaginatedResponse wraps cursor-paginated data
type CursorPaginatedResponse struct {
	Data       interface{}           `json:"data"`
	Pagination *CursorPaginationMeta `json:"pagination"`
}

// SearchParams holds search and filter parameters
type SearchParams struct {
	Query   string                 `json:"query"`
	Filters map[string]interface{} `json:"filters"`
	*PaginationParams
}

// GetSearchParams extracts search parameters from Gin context
func GetSearchParams(c *gin.Context) *SearchParams {
	params := &SearchParams{
		Query:            c.Query("q"),
		Filters:          make(map[string]interface{}),
		PaginationParams: GetPaginationParams(c),
	}

	// Extract additional filter parameters
	// Example: status, category, etc.
	if status := c.Query("status"); status != "" {
		params.Filters["status"] = status
	}

	if category := c.Query("category"); category != "" {
		params.Filters["category"] = category
	}

	if fromDate := c.Query("from_date"); fromDate != "" {
		params.Filters["from_date"] = fromDate
	}

	if toDate := c.Query("to_date"); toDate != "" {
		params.Filters["to_date"] = toDate
	}

	return params
}

// ApplyFilters applies dynamic filters to a GORM query
func ApplyFilters(db *gorm.DB, filters map[string]interface{}) *gorm.DB {
	for key, value := range filters {
		switch key {
		case "status":
			db = db.Where("status = ?", value)
		case "category":
			db = db.Where("category = ?", value)
		case "from_date":
			db = db.Where("created_at >= ?", value)
		case "to_date":
			db = db.Where("created_at <= ?", value)
		case "user_id":
			db = db.Where("user_id = ?", value)
		case "is_active":
			db = db.Where("is_active = ?", value)
			// Add more custom filters as needed
		}
	}

	return db
}

// ApplySearch applies full-text search to a GORM query
func ApplySearch(db *gorm.DB, query string, searchFields []string) *gorm.DB {
	if query == "" || len(searchFields) == 0 {
		return db
	}

	// Build OR conditions for search across multiple fields
	orConditions := db.Where("1 = 0") // Start with false condition

	for _, field := range searchFields {
		orConditions = orConditions.Or(fmt.Sprintf("%s ILIKE ?", field), "%"+query+"%")
	}

	return db.Where(orConditions)
}
