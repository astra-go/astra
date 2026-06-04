package grpc

import (
	"context"

	"github.com/astra-go/astra/examples/reference-blog/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// CommentClient wraps the gRPC CommentService for use by the API server.
// The API server delegates comment CRUD to the comment-service via gRPC.
type CommentClient struct {
	conn    *grpc.ClientConn
	service proto.CommentServiceClient
}

func NewCommentClient(addr string) (*CommentClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	return &CommentClient{
		conn:    conn,
		service: proto.NewCommentServiceClient(conn),
	}, nil
}

func (c *CommentClient) Close() error {
	return c.conn.Close()
}

// CreateComment calls the gRPC comment-service.
func (c *CommentClient) CreateComment(ctx context.Context, articleID, userID uint, parentID *uint, content string) (*proto.CommentResponse, error) {
	req := &proto.CreateCommentRequest{
		ArticleId: uint32(articleID),
		UserId:   uint32(userID),
		Content:  content,
	}
	if parentID != nil {
		pid := uint32(*parentID)
		req.ParentId = &pid
	}
	return c.service.CreateComment(ctx, req)
}

// ListComments calls the gRPC comment-service.
func (c *CommentClient) ListComments(ctx context.Context, articleID uint, page, pageSize int32) (*proto.ListCommentsResponse, error) {
	return c.service.ListComments(ctx, &proto.ListCommentsRequest{
		ArticleId: uint32(articleID),
		Page:      page,
		PageSize:  pageSize,
	})
}

// DeleteComment calls the gRPC comment-service.
func (c *CommentClient) DeleteComment(ctx context.Context, id, userID uint) error {
	_, err := c.service.DeleteComment(ctx, &proto.DeleteCommentRequest{
		Id:     uint32(id),
		UserId: uint32(userID),
	})
	return err
}

// LikeComment calls the gRPC comment-service.
func (c *CommentClient) LikeComment(ctx context.Context, id uint) error {
	_, err := c.service.LikeComment(ctx, &proto.LikeCommentRequest{
		Id: uint32(id),
	})
	return err
}
