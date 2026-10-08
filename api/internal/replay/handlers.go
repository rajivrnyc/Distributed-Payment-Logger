package replay

import (
	"context"
	"errors"
	"log"
	"time"

	"example.com/payments/internal/core"
	"example.com/payments/internal/store"
)

type EventHandler interface {
	Handle(event core.Event) error
}

// PaymentRequested Handler
type PaymentRequestedHandler struct {
	store *store.DynamoStore
}

func NewPaymentRequestedHandler(s *store.DynamoStore) *PaymentRequestedHandler {
	return &PaymentRequestedHandler{store: s}
}

func (h *PaymentRequestedHandler) Handle(event core.Event) error {
	payload := core.MustPayload(event.Payload)

	log.Printf("  [PaymentRequested] PaymentID=%s, AccountID=%s, Amount=%d %s",
		event.PaymentID, event.AccountID, payload.Amount, payload.Currency)

	ctx := context.Background()

	// Create payment record (idempotent via lastAppliedEvent check)
	err := h.store.PaymentUpsertRequested(
		ctx,
		event.PaymentID,
		event.AccountID,
		payload.Amount,
		payload.Currency,
		event.EventID,
	)
	if err != nil {
		if errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Event already applied, skipping")
			return nil
		}
		return err
	}

	// Increment pending balance (idempotent)
	err = h.store.BalanceIncPending(
		ctx,
		event.AccountID,
		payload.Currency,
		event.EventID,
		payload.Amount,
	)
	if err != nil && !errors.Is(err, store.ErrConditionalCheck) {
		return err
	}

	log.Printf("    -> Payment created and balance updated")
	return nil
}

// ============================================================================
// PaymentAuthorized Handler
// ============================================================================

type PaymentAuthorizedHandler struct {
	store *store.DynamoStore
}

func NewPaymentAuthorizedHandler(s *store.DynamoStore) *PaymentAuthorizedHandler {
	return &PaymentAuthorizedHandler{store: s}
}

func (h *PaymentAuthorizedHandler) Handle(event core.Event) error {
	log.Printf("  [PaymentAuthorized] PaymentID=%s", event.PaymentID)

	ctx := context.Background()

	err := h.store.PaymentMarkAuthorized(ctx, event.PaymentID, event.EventID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Event already applied, skipping")
			return nil
		}
		return err
	}

	log.Printf("    -> Payment authorized")
	return nil
}

// ============================================================================
// PaymentCaptured Handler
// ============================================================================

type PaymentCapturedHandler struct {
	store *store.DynamoStore
}

func NewPaymentCapturedHandler(s *store.DynamoStore) *PaymentCapturedHandler {
	return &PaymentCapturedHandler{store: s}
}

func (h *PaymentCapturedHandler) Handle(event core.Event) error {
	payload := core.MustPayload(event.Payload)

	log.Printf("  [PaymentCaptured] PaymentID=%s, CapturedAmount=%d",
		event.PaymentID, payload.CapturedAmount)

	ctx := context.Background()

	// Get payment details
	amount, accountID, _ := h.store.GetPaymentAmountAndAcct(event.PaymentID)

	// Mark payment as captured
	err := h.store.PaymentMarkCaptured(ctx, event.PaymentID, event.EventID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Event already applied, skipping")
			return nil
		}
		return err
	}

	// Move pending to available
	if accountID != "" {
		capturedAmount := payload.CapturedAmount
		if capturedAmount == 0 {
			capturedAmount = amount
		}

		err = h.store.BalanceMovePendingToAvailable(ctx, accountID, event.EventID, capturedAmount)
		if err != nil && !errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Warning: Failed to update balance: %v", err)
		}
	}

	log.Printf("    -> Payment captured")
	return nil
}

// ============================================================================
// PaymentFailed Handler
// ============================================================================

type PaymentFailedHandler struct {
	store *store.DynamoStore
}

func NewPaymentFailedHandler(s *store.DynamoStore) *PaymentFailedHandler {
	return &PaymentFailedHandler{store: s}
}

func (h *PaymentFailedHandler) Handle(event core.Event) error {
	payload := core.MustPayload(event.Payload)

	log.Printf("  [PaymentFailed] PaymentID=%s, Reason=%s",
		event.PaymentID, payload.Reason)

	ctx := context.Background()

	// Get payment details
	amount, accountID, _ := h.store.GetPaymentAmountAndAcct(event.PaymentID)

	// Mark payment as failed
	err := h.store.PaymentMarkFailed(ctx, event.PaymentID, event.EventID, payload.Reason)
	if err != nil {
		if errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Event already applied, skipping")
			return nil
		}
		return err
	}

	// Release pending balance
	if accountID != "" && amount > 0 {
		err = h.store.BalanceDecPending(ctx, accountID, event.EventID, amount)
		if err != nil && !errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Warning: Failed to update balance: %v", err)
		}
	}

	log.Printf("    -> Payment marked as failed")
	return nil
}

// ============================================================================
// AuthRequested Handler (Simple - just log for now)
// ============================================================================

type AuthRequestedHandler struct {
	store *store.DynamoStore
}

func NewAuthRequestedHandler(s *store.DynamoStore) *AuthRequestedHandler {
	return &AuthRequestedHandler{store: s}
}

func (h *AuthRequestedHandler) Handle(event core.Event) error {
	log.Printf("  [AuthRequested] PaymentID=%s", event.PaymentID)
	// This might trigger async authorization - for replay, just log
	return nil
}

// ============================================================================
// CaptureRequested Handler (Simple - just log for now)
// ============================================================================

type CaptureRequestedHandler struct {
	store *store.DynamoStore
}

func NewCaptureRequestedHandler(s *store.DynamoStore) *CaptureRequestedHandler {
	return &CaptureRequestedHandler{store: s}
}

func (h *CaptureRequestedHandler) Handle(event core.Event) error {
	log.Printf("  [CaptureRequested] PaymentID=%s", event.PaymentID)
	// This might trigger async capture - for replay, just log
	return nil
}

// ============================================================================
// FailedManually Handler
// ============================================================================

type FailedManuallyHandler struct {
	store *store.DynamoStore
}

func NewFailedManuallyHandler(s *store.DynamoStore) *FailedManuallyHandler {
	return &FailedManuallyHandler{store: s}
}

func (h *FailedManuallyHandler) Handle(event core.Event) error {
	payload := core.MustPayload(event.Payload)

	log.Printf("  [FailedManually] PaymentID=%s, Reason=%s",
		event.PaymentID, payload.Reason)

	ctx := context.Background()

	// Get payment details
	amount, accountID, _ := h.store.GetPaymentAmountAndAcct(event.PaymentID)

	// Mark payment as failed
	err := h.store.PaymentMarkFailed(ctx, event.PaymentID, event.EventID, payload.Reason)
	if err != nil {
		if errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Event already applied, skipping")
			return nil
		}
		return err
	}

	// Release pending balance
	if accountID != "" && amount > 0 {
		err = h.store.BalanceDecPending(ctx, accountID, event.EventID, amount)
		if err != nil && !errors.Is(err, store.ErrConditionalCheck) {
			log.Printf("    -> Warning: Failed to update balance: %v", err)
		}
	}

	log.Printf("    -> Payment manually failed")
	return nil
}
