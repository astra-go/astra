package handler

import (
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
)

type SearchHandler struct {
	searchService *service.SearchService
}

func NewSearchHandler(searchService *service.SearchService) *SearchHandler {
	return &SearchHandler{searchService: searchService}
}

func (h *SearchHandler) Search(c *astra.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query parameter required"})
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	result, err := h.searchService.Search(c.Request().Context(), query, page, pageSize)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"total":     result.Total,
		"page":      page,
		"page_size": pageSize,
		"articles":  result.Articles,
	})
}
