package http

import (
	"encoding/json"
	"io"
	stdhttp "net/http"
	"os"
	"time"

	"example.com/payments/internal/broker"
	"example.com/payments/internal/core"
	"example.com/payments/internal/store"
	"example.com/payments/pkg/hash"
	"example.com/payments/pkg/id"
	"example.com/payments/pkg/resp"
)

type HandlersImpl struct {
	b  broker.Broker
	st store.Store
}

func NewHandlers(b broker.Broker, st store.Store) *HandlersImpl { return &HandlersImpl{b: b, st: st} }

// DTOs

type CreatePaymentReq struct {
	AccountID        string         `json:"accountId"`
	Amount           int64          `json:"amount"`
	Currency         string         `json:"currency"`
	PaymentMethodRef string         `json:"paymentMethodRef"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type ActionReq struct {
	Amount   *int64         `json:"amount,omitempty"`
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (h *HandlersImpl) CreatePayment(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		resp.Error(w, stdhttp.StatusBadRequest, "could not read body")
		return
	}
	var req CreatePaymentReq
	if err := json.Unmarshal(body, &req); err != nil {
		resp.Error(w, stdhttp.StatusBadRequest, "invalid JSON")
		return
	}

	// Basic validation + USD-only for v1 (minor units)
	if req.AccountID == "" || req.Amount <= 0 || req.Currency == "" || req.PaymentMethodRef == "" {
		resp.Error(w, stdhttp.StatusBadRequest, "missing/invalid fields")
		return
	}
	if req.Currency != "USD" {
		resp.Error(w, stdhttp.StatusBadRequest, "only USD supported in v1")
		return
	}

	// Canonical request hash (sorted keys)
	canon, err := hash.CanonicalizeJSON(body)
	if err != nil {
		resp.Error(w, stdhttp.StatusBadRequest, "invalid JSON")
		return
	}
	reqHash := hash.SHA256Hex([]byte(canon))
	
	// Idempotency-Key is now optional
	idemKey := r.Header.Get("Idempotency-Key")
	compKey := h.st.ComposeIdemKey(idemKey, r.Method, r.URL.Path, reqHash)

	// Generate payment ID once (before any I/O)
	paymentID := id.New()
	
	// TTL: 48h for explicit header, 5s for body-hash based
	ttl := 5 * time.Second
	if idemKey != "" {
		ttl = 48 * time.Hour
	}
	
	// Try to atomically record idempotency with this payment ID
	// If it already exists, we get back the existing payment ID instead
	resultPaymentID, err := h.st.IdemPutIfAbsent(compKey, paymentID, ttl)
	if err != nil {
		resp.Error(w, stdhttp.StatusInternalServerError, "idempotency check failed")
		return
	}
	
	// If resultPaymentID != paymentID, it means another request won the race
	// Return the existing payment ID (idempotent response)
	if resultPaymentID != paymentID {
		resp.JSON(w, stdhttp.StatusAccepted, map[string]any{"paymentId": resultPaymentID})
		return
	}

	// We won the race - this is the first request with this idempotency key
	// Produce the event
	e := core.Event{
		EventID:        id.New(),
		EventType:      core.EvtPaymentRequested,
		OccurredAt:     time.Now().UTC(),
		Version:        1,
		TenantID:       "default",
		AccountID:      req.AccountID,
		PaymentID:      paymentID,
		IdempotencyKey: idemKey,
		Payload:        core.Payload{Amount: req.Amount, Currency: req.Currency},
	}

	h.b.Publish(e)
	resp.JSON(w, stdhttp.StatusAccepted, map[string]any{"paymentId": paymentID})
}

func (h *HandlersImpl) GetPayment(w stdhttp.ResponseWriter, r *stdhttp.Request, paymentID string) {
	pv, ok := h.st.GetPayment(paymentID)
	if !ok {
		resp.Error(w, stdhttp.StatusNotFound, "payment not found")
		return
	}
	resp.JSON(w, stdhttp.StatusOK, map[string]any{"status": pv.Status, "amount": pv.Amount, "currency": pv.Currency, "authorizedAt": pv.AuthorizedAt, "capturedAt": pv.CapturedAt, "failureReason": pv.FailureReason, "version": pv.Version})
}

func (h *HandlersImpl) Authorize(w stdhttp.ResponseWriter, r *stdhttp.Request, paymentID string) {
	if _, ok := h.st.GetPayment(paymentID); !ok {
		resp.Error(w, stdhttp.StatusNotFound, "payment not found")
		return
	}
	h.b.Publish(core.Event{EventID: id.New(), EventType: core.EvtPaymentAuthorized, OccurredAt: time.Now().UTC(), Version: 1, TenantID: "default", PaymentID: paymentID})
	w.WriteHeader(stdhttp.StatusAccepted)
}

func (h *HandlersImpl) Capture(w stdhttp.ResponseWriter, r *stdhttp.Request, paymentID string) {
	pv, ok := h.st.GetPayment(paymentID)
	if !ok {
		resp.Error(w, stdhttp.StatusNotFound, "payment not found")
		return
	}
	var req ActionReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	amount := pv.Amount
	if req.Amount != nil {
		amount = *req.Amount
	}
	// single-capture model: ignore further captures in processor by state (not implemented yet)
	h.b.Publish(core.Event{EventID: id.New(), EventType: core.EvtPaymentCaptured, OccurredAt: time.Now().UTC(), Version: 1, TenantID: "default", PaymentID: paymentID, AccountID: pv.AccountID, Payload: core.Payload{CapturedAmount: amount, Currency: pv.Currency}})
	w.WriteHeader(stdhttp.StatusAccepted)
}

func (h *HandlersImpl) Fail(w stdhttp.ResponseWriter, r *stdhttp.Request, paymentID string) {
	if _, ok := h.st.GetPayment(paymentID); !ok {
		resp.Error(w, stdhttp.StatusNotFound, "payment not found")
		return
	}
	var req ActionReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	reason := req.Reason
	if reason == "" {
		reason = "manual"
	}
	h.b.Publish(core.Event{EventID: id.New(), EventType: core.EvtFailedManually, OccurredAt: time.Now().UTC(), Version: 1, TenantID: "default", PaymentID: paymentID, Payload: core.Payload{Reason: reason}})
	w.WriteHeader(stdhttp.StatusAccepted)
}

func (h *HandlersImpl) GetBalance(w stdhttp.ResponseWriter, r *stdhttp.Request, accountID string) {
	if bal, ok := h.st.GetBalance(accountID); ok {
		resp.JSON(w, stdhttp.StatusOK, bal)
		return
	}
	resp.JSON(w, stdhttp.StatusOK, map[string]any{"available": 0, "pending": 0, "currency": "USD", "lastUpdated": time.Now().UTC(), "version": 0})
}

func (h *HandlersImpl) Health(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	brokerBackend := os.Getenv("BROKER_BACKEND")
	if brokerBackend == "" {
		brokerBackend = "memory"
	}

	resp.JSON(w, stdhttp.StatusOK, map[string]any{
		"status": "ok",
		"deps": map[string]any{
			"broker": brokerBackend,
		},
	})
}

func (h *HandlersImpl) Replay(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	w.WriteHeader(stdhttp.StatusAccepted)
}
