package svc

import (
	"context"
	"log"
	"os"

	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

type SMSProvider interface {
	Send(ctx context.Context, to, message string) error
}

type MockSMSProvider struct{}

func (m *MockSMSProvider) Send(_ context.Context, to, message string) error {
	log.Printf("[SMS MOCK] To: %s | Body: %s", to, message)
	return nil
}

type TwilioSMSProvider struct {
	client *twilio.RestClient
	from   string
}

func NewTwilioSMSProvider() *TwilioSMSProvider {
	accountSID := os.Getenv("TWILIO_ACCOUNT_SID")
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	from := os.Getenv("TWILIO_FROM_NUMBER")
	if accountSID == "" || authToken == "" || from == "" {
		log.Println("[SMS] Twilio credentials incomplete, SMS will not be sent")
		return nil
	}
	return &TwilioSMSProvider{
		client: twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: accountSID,
			Password: authToken,
		}),
		from: from,
	}
}

func (t *TwilioSMSProvider) Send(ctx context.Context, to, message string) error {
	if t.client == nil {
		log.Printf("[SMS] Skipping (no client): To: %s | Body: %s", to, message)
		return nil
	}
	params := &openapi.CreateMessageParams{}
	params.SetTo(to)
	params.SetFrom(t.from)
	params.SetBody(message)
	_, err := t.client.Api.CreateMessage(params)
	if err != nil {
		log.Printf("[SMS] Failed to send to %s: %v", to, err)
		return err
	}
	log.Printf("[SMS] Sent to %s", to)
	return nil
}

func NewSMSProvider() SMSProvider {
	p := NewTwilioSMSProvider()
	if p != nil {
		return p
	}
	return &MockSMSProvider{}
}
