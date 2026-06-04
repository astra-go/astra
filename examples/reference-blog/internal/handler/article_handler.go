package handler

import (
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/astra-go/astra/orm"
)

type ArticleHandler struct {
	articleService *service.ArticleService
}

func NewArticleHandler(articleService *service.ArticleService) *ArticleHandler {
	return &ArticleHandler{articleService: articleService}
}

type createArticleRequest struct {
	Title   string  `json:"title" binding:"required,max=255"`
	Content string  `json:"content" binding:"required"`
	Summary *string `json:"summary,omitempty"`
	Tags    *string `json:"tags,omitempty"`
}

type updateArticleRequest struct {
	Title   string  `json:"title" binding:"required,max=255"`
	Content string  `json:"content" binding:"required"`
	Summary *string `json:"summary,omitempty"`
	Tags    *string `json:"tags,omitempty"`
}

func (h *ArticleHandler) Create(c *astra.Ctx) error {
	var req createArticleRequest
	if err := c.BindJSON(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	userID := c.MustGet("user_id").(uint)

	article, err := h.articleService.Create(c.Request().Context(), service.CreateArticleRequest{
		Title:    req.Title,
		Content:  req.Content,
		Summary:  req.Summary,
		Tags:     req.Tags,
		AuthorID: userID,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, article)
}

func (h *ArticleHandler) GetByID(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	article, err := h.articleService.GetByID(c.Request().Context(), uint(id))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "article not found"})
	}

	return c.JSON(http.StatusOK, article)
}

func (h *ArticleHandler) List(c *astra.Ctx) error {
	page := orm.ParsePage(c)
	articles, total, err := h.articleService.ListPublished(c.Request().Context(), page)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, orm.NewPageResponse(page, total, articles))
}

func (h *ArticleHandler) ListByAuthor(c *astra.Ctx) error {
	authorID, err := strconv.ParseUint(c.Param("author_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid author_id"})
	}

	page := orm.ParsePage(c)
	articles, total, err := h.articleService.ListByAuthor(c.Request().Context(), uint(authorID), page)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, orm.NewPageResponse(page, total, articles))
}

func (h *ArticleHandler) Update(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var req updateArticleRequest
	if err := c.BindJSON(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	article, err := h.articleService.Update(c.Request().Context(), service.UpdateArticleRequest{
		ID:      uint(id),
		Title:   req.Title,
		Content: req.Content,
		Summary: req.Summary,
		Tags:    req.Tags,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, article)
}

func (h *ArticleHandler) Publish(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	if err := h.articleService.Publish(c.Request().Context(), uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "article published"})
}

func (h *ArticleHandler) Delete(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	if err := h.articleService.Delete(c.Request().Context(), uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *ArticleHandler) Like(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	if err := h.articleService.Like(c.Request().Context(), uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "liked"})
}
