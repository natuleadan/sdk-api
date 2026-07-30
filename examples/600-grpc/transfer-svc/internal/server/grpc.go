package server

import (
	"context"
	"fmt"

	transferpb "600-grpc/pb/transferpb"

	"github.com/natuleadan/sdk-api/runtime"
)

type TransferGRPCServer struct {
	transferpb.UnimplementedTransferServiceServer
	pool *runtime.PGPool
}

func NewTransferGRPCServer(pool *runtime.PGPool) *TransferGRPCServer {
	return &TransferGRPCServer{pool: pool}
}

func (s *TransferGRPCServer) GetTransfer(ctx context.Context, req *transferpb.GetTransferRequest) (*transferpb.GetTransferResponse, error) {
	var id int64
	var fromID, toID int64
	var amount float64
	var currency, status string
	err := s.pool.QueryRow(ctx,
		"SELECT id, from_account_id, to_account_id, amount, currency, status FROM transfers WHERE id = $1",
		req.TransferId).Scan(&id, &fromID, &toID, &amount, &currency, &status)
	if err != nil {
		return &transferpb.GetTransferResponse{Ok: false}, nil
	}
	return &transferpb.GetTransferResponse{
		Ok: true,
		Transfer: &transferpb.Transfer{
			Id:          fmt.Sprintf("%d", id),
			FromAccount: fmt.Sprintf("%d", fromID),
			ToAccount:   fmt.Sprintf("%d", toID),
			Amount:      amount,
			Currency:    currency,
			Status:      status,
		},
	}, nil
}
