package broker

import (
	"time"

	"example.com/payments/internal/core"
	"example.com/payments/internal/store"
)

type Broker interface{ Publish(e core.Event) error }

type inMemory struct{ ch chan core.Event }

func NewInMemory(buf int) *inMemory      { return &inMemory{ch: make(chan core.Event, buf)} }
func (b *inMemory) Publish(e core.Event) error { b.ch <- e; return nil }

func StartProcessor(b *inMemory, st store.Store) {
	go func() {
		for e := range b.ch {
			apply(st, e)
		}
	}()
}

func apply(st store.Store, e core.Event) {
	switch e.EventType {
	case core.EvtPaymentRequested:
		p := core.MustPayload(e.Payload)
		st.UpsertPaymentFromRequested(e.PaymentID, e.AccountID, p.Amount, p.Currency, e.EventID)
		st.IncPending(e.AccountID, p.Currency, p.Amount, time.Now().UTC())
	case core.EvtPaymentAuthorized:
		st.MarkAuthorized(e.PaymentID, e.EventID)
	case core.EvtPaymentCaptured:
		p := core.MustPayload(e.Payload)
		if applied := st.MarkCaptured(e.PaymentID, e.EventID); applied {
			// payout-style only when the first (and only) capture is applied
			st.MovePendingToAvailable(e.AccountID, p.Currency, p.CapturedAmount, time.Now().UTC())
		}
	case core.EvtPaymentFailed, core.EvtFailedManually:
		p := core.MustPayload(e.Payload)
		st.MarkFailed(e.PaymentID, p.Reason, e.EventID)
		amt, acct, cur := st.GetPaymentAmountAndAcct(e.PaymentID)
		if amt > 0 {
			st.DecPending(acct, cur, amt, time.Now().UTC())
		}
	}
}
