package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/reference-blog/internal/grpc"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/astra-go/astra/orm"
)

// CommentHandler delegates comment operations to the comment-service via gRPC.
type CommentHandler struct {
	localSvc    *service.CommentService  // fallback (nil when gRPC available)
	grpcClient  *grpc.CommentClient     // primary (nil when connection fails)
}

func NewCommentHandler(localSvc *service.CommentService, grpcClient *grpc.CommentClient) *CommentHandler {
	return &CommentHandler{localSvc: localSvc, grpcClient: grpcClient}
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

	if h.grpcClient != nil {
		_, err := h.grpcClient.CreateComment(c.Request().Context(), req.ArticleID, userID, req.ParentID, req.Content)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusCreated, map[string]string{"message": "comment created via gRPC"})
	}

	if h.localSvc == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "comment service unavailable"})
	}

	_, err := h.localSvc.Create(c.Request().Context(), service.CreateCommentRequest{
		ArticleID: req.ArticleID,
		UserID:    userID,
		ParentID:  req.ParentID,
		Content:   req.Content,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]string{"message": "comment created"})
}

func (h *CommentHandler) ListByArticle(c *astra.Ctx) error {
	articleID, err := strconv.ParseUint(c.Param("article_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid article_id"})
	}

	page := orm.ParsePage(c)

	if h.grpcClient != nil {
		resp, err := h.grpcClient.ListComments(c.Request().Context(), uint(articleID), int32(page.Page), int32(page.PageSize))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"total":     resp.Total,
			"page":      resp.Page,
			"page_size": resp.PageSize,
			"comments":  resp.Comments,
		})
	}

	if h.localSvc == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "comment service unavailable"})
	}

	comments, total, err := h.localSvc.ListByArticle(c.Request().Context(), uint(articleID), page)
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

	if h.grpcClient != nil {
		if err := h.grpcClient.DeleteComment(c.Request().Context(), uint(id), userID); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.NoContent(http.StatusNoContent)
	}

	if h.localSvc == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "comment service unavailable"})
	}

	if err := h.localSvc.Delete(c.Request().Context(), uint(id), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *CommentHandler) Like(c *astra.Ctx) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	if h.grpcClient != nil {
		if err := h.grpcClient.LikeComment(c.Request().Context(), uint(id)); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "liked via gRPC"})
	}

	if h.localSvc == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "comment service unavailable"})
	}

	if err := h.localSvc.Like(context.Background(), uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "liked"})
}
