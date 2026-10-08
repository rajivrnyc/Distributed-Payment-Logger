package store

import (
	"sync"
	"time"

	"example.com/payments/internal/core"
)

type memory struct {
	mu       sync.RWMutex
	payments map[string]*core.PaymentView
	balances map[string]*core.BalanceView
	idem     map[string]idemEntry // compKey -> entry
}

type idemEntry struct {
	paymentID string
	expireAt  time.Time
}

func NewMemory() *memory {
	return &memory{payments: make(map[string]*core.PaymentView), balances: make(map[string]*core.BalanceView), idem: make(map[string]idemEntry)}
}

// idempotency
func (m *memory) ComposeIdemKey(idemKey, method, path, requestHash string) string {
	if idemKey != "" {
		return "key:" + idemKey
	}
	return "hash:" + requestHash
}

func (m *memory) IdemLookup(compKey string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.idem[compKey]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expireAt) {
		delete(m.idem, compKey)
		return "", false
	}
	return entry.paymentID, true
}

func (m *memory) IdemRecord(compKey, paymentID string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idem[compKey] = idemEntry{paymentID: paymentID, expireAt: time.Now().Add(ttl)}
}

func (m *memory) IdemPutIfAbsent(compKey, paymentID string, ttl time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check if already exists
	if entry, ok := m.idem[compKey]; ok {
		if time.Now().Before(entry.expireAt) {
			// Return existing payment ID
			return entry.paymentID, nil
		}
		// Expired, remove it
		delete(m.idem, compKey)
	}
	
	// Insert new entry
	m.idem[compKey] = idemEntry{paymentID: paymentID, expireAt: time.Now().Add(ttl)}
	return paymentID, nil
}

// payments
func (m *memory) GetPayment(paymentID string) (core.PaymentView, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pv, ok := m.payments[paymentID]
	if !ok {
		return core.PaymentView{}, false
	}
	return *pv, true
}

func (m *memory) UpsertPaymentFromRequested(paymentID, accountID string, amount int64, currency, eventID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pv := &core.PaymentView{PaymentID: paymentID, AccountID: accountID, Amount: amount, Currency: currency, Status: core.StatusRequested, Version: 1, LastAppliedEventID: eventID}
	m.payments[paymentID] = pv
}

func (m *memory) MarkAuthorized(paymentID, eventID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pv := m.payments[paymentID]
	if pv == nil {
		return
	}
	if pv.Status == core.StatusCaptured || pv.Status == core.StatusFailed {
		return
	}
	now := time.Now().UTC()
	pv.Status = core.StatusAuthorized
	pv.AuthorizedAt = &now
	pv.Version++
	pv.LastAppliedEventID = eventID
}

func (m *memory) MarkCaptured(paymentID, eventID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	pv := m.payments[paymentID]
	if pv == nil {
		return false
	}

	// Block capture if payment has not been authorized yet.
	if pv.Status != core.StatusAuthorized {
		return false
	}

	// Enforce single-capture: ignore if already captured or failed
	if pv.Status == core.StatusCaptured || pv.Status == core.StatusFailed {
		return false
	}

	now := time.Now().UTC()
	pv.Status = core.StatusCaptured
	pv.CapturedAt = &now
	pv.Version++
	pv.LastAppliedEventID = eventID
	return true
}

func (m *memory) MarkFailed(paymentID, reason, eventID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pv := m.payments[paymentID]
	if pv == nil {
		return
	}
	// If already captured, we do not overwrite success with failure.
	if pv.Status == core.StatusCaptured {
		return
	}
	pv.Status = core.StatusFailed
	pv.FailureReason = reason
	pv.Version++
	pv.LastAppliedEventID = eventID
}

// balances
func (m *memory) GetBalance(accountID string) (core.BalanceView, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.balances[accountID]
	if !ok {
		return core.BalanceView{}, false
	}
	return *b, true
}

func (m *memory) ensureBalance(accountID, currency string) *core.BalanceView {
	b := m.balances[accountID]
	if b == nil {
		b = &core.BalanceView{AccountID: accountID, Currency: currency}
		m.balances[accountID] = b
	}
	return b
}

func (m *memory) IncPending(accountID, currency string, amount int64, ts time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.ensureBalance(accountID, currency)
	b.Pending += amount
	b.Version++
	b.LastUpdated = ts
}
func (m *memory) DecPending(accountID, currency string, amount int64, ts time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.ensureBalance(accountID, currency)
	b.Pending -= amount
	b.Version++
	b.LastUpdated = ts
}
func (m *memory) MovePendingToAvailable(accountID, currency string, amount int64, ts time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.ensureBalance(accountID, currency)
	b.Pending -= amount
	b.Available -= amount
	b.Version++
	b.LastUpdated = ts
}

// GetPaymentAmountAndAcct returns the payment's amount (minor units), accountId, and currency.
// If the payment doesn't exist, it returns zero values.
func (m *memory) GetPaymentAmountAndAcct(paymentID string) (int64, string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pv := m.payments[paymentID]
	if pv == nil {
		return 0, "", ""
	}
	return pv.Amount, pv.AccountID, pv.Currency
}
