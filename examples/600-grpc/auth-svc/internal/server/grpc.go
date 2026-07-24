package server

import (
	"context"

	authpb "600-grpc/pb/authpb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthGRPCServer struct {
	authpb.UnimplementedAuthServiceServer
	pool *pgxpool.Pool
}

func NewAuthGRPCServer(pool *pgxpool.Pool) *AuthGRPCServer {
	return &AuthGRPCServer{pool: pool}
}

func (s *AuthGRPCServer) DeductCredit(ctx context.Context, req *authpb.DeductCreditRequest) (*authpb.DeductCreditResponse, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET credits = credits - $1 WHERE id = $2 AND credits >= $1`, req.Amount, req.UserId)
	if err != nil || tag.RowsAffected() == 0 {
		return &authpb.DeductCreditResponse{Ok: false}, nil
	}
	var balance int32
	_ = s.pool.QueryRow(ctx, `SELECT credits FROM users WHERE id = $1`, req.UserId).Scan(&balance)
	return &authpb.DeductCreditResponse{Ok: true, Balance: balance}, nil
}

func (s *AuthGRPCServer) VerifyToken(ctx context.Context, req *authpb.VerifyTokenRequest) (*authpb.VerifyTokenResponse, error) {
	return &authpb.VerifyTokenResponse{Valid: true}, nil
}
