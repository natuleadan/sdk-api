package handler

import (
	"tickets/internal/svc"

	"github.com/natuleadan/sdk-api/runtime"
)

func RegisterRoutes(s *runtime.Service, svcCtx *svc.ServiceContext) {
	s.WithRest("createOrder", CreateOrder(svcCtx))
	s.WithRest("getOrder", GetOrder(svcCtx))
	s.WithRest("validatePayment", ValidatePayment(svcCtx))
	s.WithRest("resetStock", ResetStock(svcCtx))
	s.WithRest("dailyReport", DailyReport(svcCtx))
	s.WithRest("batchCompleteWebhook", BatchCompleteWebhook(svcCtx))
	s.WithAsync("processBatch", ProcessBatch(svcCtx))

	s.WithExit("onOrderConfirmed", svcCtx.OnOrderConfirmed)
	s.WithExit("onBatchPayment", svcCtx.OnBatchPayment)
	s.WithExit("onValidatePayment", svcCtx.OnValidatePayment)
	s.WithCron("onDailyReport", svcCtx.OnDailyReport)
}
