package server

import (
	"context"
	"fmt"

	accountpb "600-grpc/pb/accountpb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountGRPCServer struct {
	accountpb.UnimplementedAccountServiceServer
	pool *pgxpool.Pool
}

func NewAccountGRPCServer(pool *pgxpool.Pool) *AccountGRPCServer {
	return &AccountGRPCServer{pool: pool}
}

func (s *AccountGRPCServer) DeductBalance(ctx context.Context, req *accountpb.DeductBalanceRequest) (*accountpb.DeductBalanceResponse, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE accounts SET balance = balance - $1 WHERE id = $2 AND balance >= $1 AND status = 'active'`,
		req.Amount, req.AccountId)
	if err != nil {
		return &accountpb.DeductBalanceResponse{Ok: false, Error: err.Error()}, nil
	}
	if tag.RowsAffected() == 0 {
		return &accountpb.DeductBalanceResponse{Ok: false, Error: "insufficient funds or inactive account"}, nil
	}
	var balance float64
	_ = s.pool.QueryRow(ctx, `SELECT balance FROM accounts WHERE id = $1`, req.AccountId).Scan(&balance)
	_, _ = s.pool.Exec(ctx, `INSERT INTO transactions (account_id, type, amount, reference_type, reference_id) VALUES ($1,'deduct',$2,'transfer',$3)`,
		req.AccountId, req.Amount, req.IdempotencyKey)
	return &accountpb.DeductBalanceResponse{Ok: true, NewBalance: balance}, nil
}

func (s *AccountGRPCServer) CreditBalance(ctx context.Context, req *accountpb.CreditBalanceRequest) (*accountpb.CreditBalanceResponse, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE accounts SET balance = balance + $1 WHERE id = $2 AND status = 'active'`,
		req.Amount, req.AccountId)
	if err != nil || tag.RowsAffected() == 0 {
		return &accountpb.CreditBalanceResponse{Ok: false, Error: fmt.Sprintf("credit failed: %v", err)}, nil
	}
	var balance float64
	_ = s.pool.QueryRow(ctx, `SELECT balance FROM accounts WHERE id = $1`, req.AccountId).Scan(&balance)
	_, _ = s.pool.Exec(ctx, `INSERT INTO transactions (account_id, type, amount, reference_type, reference_id) VALUES ($1,'credit',$2,'transfer',$3)`,
		req.AccountId, req.Amount, req.IdempotencyKey)
	return &accountpb.CreditBalanceResponse{Ok: true, NewBalance: balance}, nil
}

func (s *AccountGRPCServer) CreateAccount(ctx context.Context, req *accountpb.CreateAccountRequest) (*accountpb.CreateAccountResponse, error) {
	return &accountpb.CreateAccountResponse{Ok: false, Error: "use REST endpoint"}, nil
}

func (s *AccountGRPCServer) GetBalance(ctx context.Context, req *accountpb.GetBalanceRequest) (*accountpb.GetBalanceResponse, error) {
	var balance float64
	err := s.pool.QueryRow(ctx, `SELECT balance FROM accounts WHERE id = $1`, req.AccountId).Scan(&balance)
	if err != nil {
		return &accountpb.GetBalanceResponse{Ok: false}, nil
	}
	return &accountpb.GetBalanceResponse{Ok: true, Balance: balance}, nil
}
