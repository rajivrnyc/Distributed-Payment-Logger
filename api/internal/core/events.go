package core

import "time"

type EventType string

const (
	EvtPaymentRequested  EventType = "PaymentRequested"
	EvtPaymentAuthorized EventType = "PaymentAuthorized"
	EvtPaymentCaptured   EventType = "PaymentCaptured"
	EvtPaymentFailed     EventType = "PaymentFailed"
	EvtAuthRequested     EventType = "PaymentAuthorizationRequested"
	EvtCaptureRequested  EventType = "PaymentCaptureRequested"
	EvtFailedManually    EventType = "PaymentFailedManually"
)

type Event struct {
	EventID        string    `json:"eventId"`
	EventType      EventType `json:"eventType"`
	OccurredAt     time.Time `json:"occurredAt"`
	Version        int       `json:"version"`
	TenantID       string    `json:"tenantId"`
	AccountID      string    `json:"accountId"`
	PaymentID      string    `json:"paymentId"`
	IdempotencyKey string    `json:"idempotencyKey"`
	Payload        Payload   `json:"payload"` // FIX: typed payload
}

// payloads

type Payload struct {
	Amount         int64  `json:"amount,omitempty"`
	Currency       string `json:"currency,omitempty"`
	CapturedAmount int64  `json:"capturedAmount,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func MustPayload(v any) Payload { p, _ := v.(Payload); return p }
