package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	// "strconv"
	"strings"
	"time"

	"example.com/payments/internal/core"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	ErrPaymentNotFound  = errors.New("payment not found")
	ErrBalanceNotFound  = errors.New("balance not found")
	ErrConditionalCheck = errors.New("conditional write failed")
)

type DynamoStore struct {
	ddb              *ddb.Client
	tableIdempotency string
	tablePayments    string
	tableBalances    string
}

type DynamoConfig struct {
	Region           string
	TableIdempotency string
	TablePayments    string
	TableBalances    string
}

func NewDynamo(ctx context.Context, cfg DynamoConfig) (*DynamoStore, error) {
	if cfg.Region == "" {
		cfg.Region = os.Getenv("AWS_REGION")
	}
	if cfg.TableIdempotency == "" || cfg.TablePayments == "" || cfg.TableBalances == "" {
		return nil, fmt.Errorf("missing DDB table envs: DDB_TABLE_IDEMPOTENCY/DDB_TABLE_PAYMENTS/DDB_TABLE_BALANCES")
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		return nil, err
	}
	return &DynamoStore{
		ddb:              ddb.NewFromConfig(awsCfg),
		tableIdempotency: cfg.TableIdempotency,
		tablePayments:    cfg.TablePayments,
		tableBalances:    cfg.TableBalances,
	}, nil
}

type IdemRecord struct {
	Key       string
	ResultRef string
	TTL       int64
	CreatedAt string
}

func (s *DynamoStore) idemPutIfAbsentInternal(ctx context.Context, rec IdemRecord) (*IdemRecord, error) {
	if rec.TTL == 0 {
		rec.TTL = time.Now().Add(48 * time.Hour).Unix()
	}
	_, err := s.ddb.PutItem(ctx, &ddb.PutItemInput{
		TableName: aws.String(s.tableIdempotency),
		Item: map[string]types.AttributeValue{
			"id":        &types.AttributeValueMemberS{Value: rec.Key},
			"resultRef": &types.AttributeValueMemberS{Value: rec.ResultRef},
			"createdAt": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
			"ttl":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", rec.TTL)},
		},
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	var ccfe *types.ConditionalCheckFailedException
	if err != nil {
		if errors.As(err, &ccfe) {
			return s.IdemGet(ctx, rec.Key)
		}
		return nil, err
	}
	return &rec, nil
}

func (s *DynamoStore) IdemGet(ctx context.Context, key string) (*IdemRecord, error) {
	out, err := s.ddb.GetItem(ctx, &ddb.GetItemInput{
		TableName:      aws.String(s.tableIdempotency),
		Key:            map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: key}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}
	return &IdemRecord{
		Key:       key,
		ResultRef: getS(out.Item["resultRef"]),
		CreatedAt: getS(out.Item["createdAt"]),
	}, nil
}

type PaymentItem struct {
	PK               string
	AccountID        string
	Amount           int64
	Currency         string
	Status           string
	AuthorizedAt     string
	CapturedAt       string
	FailureReason    string
	Version          int64
	LastAppliedEvent string
}

func (s *DynamoStore) PaymentUpsertRequested(ctx context.Context, paymentId, accountId string, amount int64, currency, eventId string) error {
	_, err := s.ddb.PutItem(ctx, &ddb.PutItemInput{
		TableName: aws.String(s.tablePayments),
		Item: map[string]types.AttributeValue{
			"pk":               &types.AttributeValueMemberS{Value: "PAYMENT#" + paymentId},
			"accountId":        &types.AttributeValueMemberS{Value: accountId},
			"amount":           &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", amount)},
			"currency":         &types.AttributeValueMemberS{Value: currency},
			"status":           &types.AttributeValueMemberS{Value: "REQUESTED"},
			"version":          &types.AttributeValueMemberN{Value: "1"},
			"lastAppliedEvent": &types.AttributeValueMemberS{Value: eventId},
		},
		ConditionExpression:       aws.String("attribute_not_exists(pk) OR lastAppliedEvent <> :eid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":eid": &types.AttributeValueMemberS{Value: eventId}},
	})
	return classifyCCF(err)
}

func (s *DynamoStore) PaymentMarkAuthorized(ctx context.Context, paymentId, eventId string, at time.Time) error {
	_, err := s.ddb.UpdateItem(ctx, &ddb.UpdateItemInput{
		TableName:                aws.String(s.tablePayments),
		Key:                      keyPayment(paymentId),
		UpdateExpression:         aws.String("SET #st = :st, authorizedAt = :ts, version = if_not_exists(version, :z) + :o, lastAppliedEvent = :eid"),
		ConditionExpression:      aws.String("attribute_not_exists(lastAppliedEvent) OR lastAppliedEvent <> :eid"),
		ExpressionAttributeNames: map[string]string{"#st": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":st":  &types.AttributeValueMemberS{Value: "AUTHORIZED"},
			":ts":  &types.AttributeValueMemberS{Value: at.UTC().Format(time.RFC3339)},
			":eid": &types.AttributeValueMemberS{Value: eventId},
			":o":   &types.AttributeValueMemberN{Value: "1"},
			":z":   &types.AttributeValueMemberN{Value: "0"},
		},
	})
	return classifyCCF(err)
}

func (s *DynamoStore) PaymentMarkCaptured(ctx context.Context, paymentId, eventId string, at time.Time) error {
	_, err := s.ddb.UpdateItem(ctx, &ddb.UpdateItemInput{
		TableName:                aws.String(s.tablePayments),
		Key:                      keyPayment(paymentId),
		UpdateExpression:         aws.String("SET #st = :st, capturedAt = :ts, version = if_not_exists(version, :z) + :o, lastAppliedEvent = :eid"),
		ConditionExpression:      aws.String("attribute_not_exists(lastAppliedEvent) OR lastAppliedEvent <> :eid"),
		ExpressionAttributeNames: map[string]string{"#st": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":st":  &types.AttributeValueMemberS{Value: "CAPTURED"},
			":ts":  &types.AttributeValueMemberS{Value: at.UTC().Format(time.RFC3339)},
			":eid": &types.AttributeValueMemberS{Value: eventId},
			":o":   &types.AttributeValueMemberN{Value: "1"},
			":z":   &types.AttributeValueMemberN{Value: "0"},
		},
	})
	return classifyCCF(err)
}

func (s *DynamoStore) PaymentMarkFailed(ctx context.Context, paymentId, eventId, reason string) error {
	_, err := s.ddb.UpdateItem(ctx, &ddb.UpdateItemInput{
		TableName:                aws.String(s.tablePayments),
		Key:                      keyPayment(paymentId),
		UpdateExpression:         aws.String("SET #st = :st, failureReason = :rsn, version = if_not_exists(version, :z) + :o, lastAppliedEvent = :eid"),
		ConditionExpression:      aws.String("attribute_not_exists(lastAppliedEvent) OR lastAppliedEvent <> :eid"),
		ExpressionAttributeNames: map[string]string{"#st": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":st":  &types.AttributeValueMemberS{Value: "FAILED"},
			":rsn": &types.AttributeValueMemberS{Value: reason},
			":eid": &types.AttributeValueMemberS{Value: eventId},
			":o":   &types.AttributeValueMemberN{Value: "1"},
			":z":   &types.AttributeValueMemberN{Value: "0"},
		},
	})
	return classifyCCF(err)
}

func (s *DynamoStore) PaymentGet(ctx context.Context, paymentId string) (*PaymentItem, error) {
	out, err := s.ddb.GetItem(ctx, &ddb.GetItemInput{
		TableName:      aws.String(s.tablePayments),
		Key:            keyPayment(paymentId),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, ErrPaymentNotFound
	}
	return mapPayment(out.Item), nil
}

type BalanceItem struct {
	PK               string
	AccountID        string
	Available        int64
	Pending          int64
	Currency         string
	Version          int64
	LastAppliedEvent string
}

func (s *DynamoStore) BalanceIncPending(ctx context.Context, accountId, currency, eventId string, delta int64) error {
	_, err := s.ddb.UpdateItem(ctx, &ddb.UpdateItemInput{
		TableName:           aws.String(s.tableBalances),
		Key:                 map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "ACCOUNT#" + accountId}, "sk": &types.AttributeValueMemberS{Value: "BALANCE"}},
		UpdateExpression:    aws.String("SET currency = if_not_exists(currency, :cur), pending = if_not_exists(pending, :z) + :d, version = if_not_exists(version, :z) + :o, lastAppliedEvent = :eid"),
		ConditionExpression: aws.String("attribute_not_exists(lastAppliedEvent) OR lastAppliedEvent <> :eid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":cur": &types.AttributeValueMemberS{Value: currency},
			":d":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", delta)},
			":eid": &types.AttributeValueMemberS{Value: eventId},
			":o":   &types.AttributeValueMemberN{Value: "1"},
			":z":   &types.AttributeValueMemberN{Value: "0"},
		},
	})
	return classifyCCF(err)
}

func (s *DynamoStore) BalanceDecPending(ctx context.Context, accountId, eventId string, delta int64) error {
	_, err := s.ddb.UpdateItem(ctx, &ddb.UpdateItemInput{
		TableName:           aws.String(s.tableBalances),
		Key:                 map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "ACCOUNT#" + accountId}, "sk": &types.AttributeValueMemberS{Value: "BALANCE"}},
		UpdateExpression:    aws.String("SET pending = if_not_exists(pending, :z) - :d, version = if_not_exists(version, :z) + :o, lastAppliedEvent = :eid"),
		ConditionExpression: aws.String("attribute_not_exists(lastAppliedEvent) OR lastAppliedEvent <> :eid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":d":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", delta)},
			":eid": &types.AttributeValueMemberS{Value: eventId},
			":o":   &types.AttributeValueMemberN{Value: "1"},
			":z":   &types.AttributeValueMemberN{Value: "0"},
		},
	})
	return classifyCCF(err)
}

func (s *DynamoStore) BalanceMovePendingToAvailable(ctx context.Context, accountId, eventId string, delta int64) error {
	_, err := s.ddb.UpdateItem(ctx, &ddb.UpdateItemInput{
		TableName:           aws.String(s.tableBalances),
		Key:                 map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "ACCOUNT#" + accountId}, "sk": &types.AttributeValueMemberS{Value: "BALANCE"}},
		UpdateExpression:    aws.String("SET available = if_not_exists(available, :z) + :d, pending = if_not_exists(pending, :z) - :d, version = if_not_exists(version, :z) + :o, lastAppliedEvent = :eid"),
		ConditionExpression: aws.String("attribute_not_exists(lastAppliedEvent) OR lastAppliedEvent <> :eid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":d":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", delta)},
			":eid": &types.AttributeValueMemberS{Value: eventId},
			":o":   &types.AttributeValueMemberN{Value: "1"},
			":z":   &types.AttributeValueMemberN{Value: "0"},
		},
	})
	return classifyCCF(err)
}

func (s *DynamoStore) BalanceGet(ctx context.Context, accountId string) (*BalanceItem, error) {
	out, err := s.ddb.GetItem(ctx, &ddb.GetItemInput{
		TableName:      aws.String(s.tableBalances),
		Key:            map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "ACCOUNT#" + accountId}, "sk": &types.AttributeValueMemberS{Value: "BALANCE"}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, ErrBalanceNotFound
	}
	return mapBalance(out.Item), nil
}

// ===== Store interface compatibility =====

func (s *DynamoStore) ComposeIdemKey(idemKey, method, path, requestHash string) string {
	if idemKey != "" {
		return "key:" + idemKey
	}
	return "hash:" + requestHash
}

func (s *DynamoStore) IdemLookup(compKey string) (string, bool) {
	rec, err := s.IdemGet(context.TODO(), compKey)
	if err != nil || rec == nil {
		return "", false
	}
	return rec.ResultRef, true
}

func (s *DynamoStore) IdemRecord(compKey, paymentID string, ttl time.Duration) {
	_, _ = s.idemPutIfAbsentInternal(context.TODO(), IdemRecord{
		Key:       compKey,
		ResultRef: paymentID,
		TTL:       time.Now().Add(ttl).Unix(),
	})
}

// IdemPutIfAbsent atomically tries to insert idempotency record
// Returns the payment ID that is associated with this key (either the one we just inserted, or an existing one)
func (s *DynamoStore) IdemPutIfAbsent(compKey, paymentID string, ttl time.Duration) (string, error) {
	rec, err := s.idemPutIfAbsentInternal(context.TODO(), IdemRecord{
		Key:       compKey,
		ResultRef: paymentID,
		TTL:       time.Now().Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	if rec == nil {
		return "", fmt.Errorf("unexpected nil record")
	}
	return rec.ResultRef, nil
}

func (s *DynamoStore) GetPayment(paymentID string) (core.PaymentView, bool) {
	it, err := s.PaymentGet(context.TODO(), paymentID)
	if err != nil || it == nil {
		return core.PaymentView{}, false
	}
	return core.PaymentView{
		PaymentID:          trimPrefix(it.PK, "PAYMENT#"),
		AccountID:          it.AccountID,
		Amount:             it.Amount,
		Currency:           it.Currency,
		Status:             core.PaymentStatus(strings.ToLower(it.Status)),
		AuthorizedAt:       parseRFC3339Ptr(it.AuthorizedAt),
		CapturedAt:         parseRFC3339Ptr(it.CapturedAt),
		FailureReason:      it.FailureReason,
		Version:            int(it.Version),
		LastAppliedEventID: it.LastAppliedEvent,
	}, true
}

func (s *DynamoStore) UpsertPaymentFromRequested(paymentID, accountID string, amount int64, currency, eventID string) {
	_ = s.PaymentUpsertRequested(context.TODO(), paymentID, accountID, amount, currency, eventID)
}

func (s *DynamoStore) MarkAuthorized(paymentID, eventID string) {
	_ = s.PaymentMarkAuthorized(context.TODO(), paymentID, eventID, time.Now().UTC())
}

func (s *DynamoStore) MarkCaptured(paymentID, eventID string) bool {
	err := s.PaymentMarkCaptured(context.TODO(), paymentID, eventID, time.Now().UTC())
	return err == nil || errors.Is(err, ErrConditionalCheck)
}

func (s *DynamoStore) MarkFailed(paymentID, reason, eventID string) {
	_ = s.PaymentMarkFailed(context.TODO(), paymentID, eventID, reason)
}

func (s *DynamoStore) GetPaymentAmountAndAcct(paymentID string) (int64, string, string) {
	it, err := s.PaymentGet(context.TODO(), paymentID)
	if err != nil || it == nil {
		return 0, "", ""
	}
	return it.Amount, it.AccountID, it.Currency
}

func (s *DynamoStore) GetBalance(accountID string) (core.BalanceView, bool) {
	it, err := s.BalanceGet(context.TODO(), accountID)
	if err != nil || it == nil {
		return core.BalanceView{}, false
	}
	return core.BalanceView{
		AccountID:   accountID,
		Available:   it.Available,
		Pending:     it.Pending,
		Currency:    it.Currency,
		LastUpdated: time.Now().UTC(), // you’re not persisting a timestamp; this keeps the API non-nil
		Version:     int(it.Version),
	}, true
}

func parseRFC3339Ptr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

func (s *DynamoStore) IncPending(accountID, currency string, amount int64, ts time.Time) {
	eid := "BAL_INC:" + fmt.Sprintf("%d", ts.UnixNano())
	_ = s.BalanceIncPending(context.TODO(), accountID, currency, eid, amount)
}

func (s *DynamoStore) DecPending(accountID, currency string, amount int64, ts time.Time) {
	eid := "BAL_DEC:" + fmt.Sprintf("%d", ts.UnixNano())
	_ = s.BalanceDecPending(context.TODO(), accountID, eid, amount)
}

func (s *DynamoStore) MovePendingToAvailable(accountID, currency string, amount int64, ts time.Time) {
	eid := "BAL_MOVE:" + fmt.Sprintf("%d", ts.UnixNano())
	_ = s.BalanceMovePendingToAvailable(context.TODO(), accountID, eid, amount)
}

// ---------- helpers ----------

func keyPayment(paymentId string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "PAYMENT#" + paymentId}}
}

func mapPayment(m map[string]types.AttributeValue) *PaymentItem {
	return &PaymentItem{
		PK:               getS(m["pk"]),
		AccountID:        getS(m["accountId"]),
		Amount:           getI(m["amount"]),
		Currency:         getS(m["currency"]),
		Status:           getS(m["status"]),
		AuthorizedAt:     getS(m["authorizedAt"]),
		CapturedAt:       getS(m["capturedAt"]),
		FailureReason:    getS(m["failureReason"]),
		Version:          getI(m["version"]),
		LastAppliedEvent: getS(m["lastAppliedEvent"]),
	}
}

func mapBalance(m map[string]types.AttributeValue) *BalanceItem {
	return &BalanceItem{
		PK:               getS(m["pk"]),
		AccountID:        trimPrefix(getS(m["pk"]), "ACCOUNT#"),
		Available:        getI(m["available"]),
		Pending:          getI(m["pending"]),
		Currency:         getS(m["currency"]),
		Version:          getI(m["version"]),
		LastAppliedEvent: getS(m["lastAppliedEvent"]),
	}
}

func getS(v types.AttributeValue) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

func getI(v types.AttributeValue) int64 {
	if v == nil {
		return 0
	}
	if n, ok := v.(*types.AttributeValueMemberN); ok {
		var x int64
		fmt.Sscanf(n.Value, "%d", &x)
		return x
	}
	return 0
}

func trimPrefix(s, p string) string {
	if len(s) >= len(p) && s[:len(p)] == p {
		return s[len(p):]
	}
	return s
}

func classifyCCF(err error) error {
	if err == nil {
		return nil
	}
	var ccfe *types.ConditionalCheckFailedException
	if errors.As(err, &ccfe) {
		return ErrConditionalCheck
	}
	return err
}
