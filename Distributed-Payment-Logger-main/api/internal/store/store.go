package store

import (
	"time"

	"example.com/payments/internal/core"
)

type Store interface {
	// idempotency with TTL
	ComposeIdemKey(idemKey, method, path, requestHash string) string
	IdemLookup(compKey string) (paymentID string, ok bool)
	IdemRecord(compKey, paymentID string, ttl time.Duration)
	IdemPutIfAbsent(compKey, paymentID string, ttl time.Duration) (resultPaymentID string, err error)

	// payments
	GetPayment(paymentID string) (core.PaymentView, bool)
	UpsertPaymentFromRequested(paymentID, accountID string, amount int64, currency, eventID string)
	MarkAuthorized(paymentID, eventID string)
	// MarkCaptured returns true if this was the first (and only) capture applied; false if ignored.
	MarkCaptured(paymentID, eventID string) bool
	MarkFailed(paymentID, reason, eventID string)
	GetPaymentAmountAndAcct(paymentID string) (amount int64, accountID, currency string)

	// balances
	GetBalance(accountID string) (core.BalanceView, bool)
	IncPending(accountID, currency string, amount int64, ts time.Time)
	DecPending(accountID, currency string, amount int64, ts time.Time)
	MovePendingToAvailable(accountID, currency string, amount int64, ts time.Time)
}
