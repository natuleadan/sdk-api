.PHONY: proto build docker up down test test-f1 test-f3 test-f4 test-f5

proto:
	protoc --proto_path=proto --go_out=. --go_opt=module=600-grpc --go-grpc_out=. --go-grpc_opt=module=600-grpc proto/*.proto

auth: proto
	go build -o /dev/null ./auth-svc/cmd/

url: proto
	go build -o /dev/null ./url-svc/cmd/

file: proto
	go build -o /dev/null ./file-svc/cmd/

ticket: proto
	go build -o /dev/null ./ticket-svc/cmd/

account: proto
	go build -o /dev/null ./account-svc/cmd/

transfer: proto
	go build -o /dev/null ./transfer-svc/cmd/

fraud: proto
	go build -o /dev/null ./fraud-svc/cmd/

receipt: proto
	go build -o /dev/null ./receipt-svc/cmd/

build: auth url file ticket account transfer fraud receipt
	@echo "✅ all services build"

docker:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

test:
	go test -tags=integration -count=1 -timeout=300s ./tests/...

test-f1:
	go test -tags=integration -count=1 -timeout=120s -run "TestIdempot_|TestConc_|TestE2E_|TestSec_" ./tests/...

test-f3:
	go test -tags=integration -count=1 -timeout=120s -run "TestSer_|TestTime_|TestObs_" ./tests/...

test-f4:
	go test -tags=integration -count=1 -timeout=180s -run "TestEv_|TestBroker_|TestCons_|TestScale_" ./tests/...

test-f5:
	go test -tags=integration -count=1 -timeout=180s -run "TestCB_|TestRetry_|TestRecovery_|TestDLQ_|TestOutbox_" ./tests/...
