package core

import "time"

type PaymentStatus string

const (
	StatusRequested  PaymentStatus = "requested"
	StatusAuthorized PaymentStatus = "authorized"
	StatusCaptured   PaymentStatus = "captured"
	StatusFailed     PaymentStatus = "failed"
)

type PaymentView struct {
	PaymentID          string        `json:"paymentId"`
	AccountID          string        `json:"accountId"`
	Amount             int64         `json:"amount"`
	Currency           string        `json:"currency"`
	Status             PaymentStatus `json:"status"`
	AuthorizedAt       *time.Time    `json:"authorizedAt,omitempty"`
	CapturedAt         *time.Time    `json:"capturedAt,omitempty"`
	FailureReason      string        `json:"failureReason,omitempty"`
	Version            int           `json:"version"`
	LastAppliedEventID string        `json:"-"`
}

type BalanceView struct {
	AccountID   string    `json:"accountId"`
	Available   int64     `json:"available"`
	Pending     int64     `json:"pending"`
	Currency    string    `json:"currency"`
	LastUpdated time.Time `json:"lastUpdated"`
	Version     int       `json:"version"`
}
