package handler

import (
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/astra-go/astra/orm"
)

type CommentHandler struct {
	commentService *service.CommentService
}

func NewCommentHandler(commentService *service.CommentService) *CommentHandler {
	return &CommentHandler{commentService: commentService}
}

type createCommentRequest struct {
	ArticleID uint   `json:"article_id" binding:"required"`
	ParentID  *uint  `json:"parent_id,omitempty"`
	Content   string `json:"content" binding:"required,max=5000"`
}

func (h *CommentHandler) Create(c *astra.Ctx) error {
	var req createCommentRequest
	if err := c.BindJSON(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	userID := c.MustGet("user_id").(uint)

	comment, err := h.commentService.Create(c.Request().Context(), service.CreateCommentRequest{
		ArticleID: req.ArticleID,
		UserID:    userID,
		ParentID:  req.ParentID,
		Content:   req.Content,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, comment)
}

func (h *CommentHandler) ListByArticle(c *astra.Ctx) error {
	articleID, err := strconv.ParseUint(c.Param("article_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid article_id"})
	}

	page := orm.ParsePage(c)
	comments, total, err := h.commentService.ListByArticle(c.Request().Context(), uint(articleID), page)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, orm.NewPageResponse(page, total, comments))
}

func (h *CommentHandler) Delete(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	userID := c.MustGet("user_id").(uint)

	if err := h.commentService.Delete(c.Request().Context(), uint(id), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *CommentHandler) Like(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	if err := h.commentService.Like(c.Request().Context(), uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "liked"})
}
