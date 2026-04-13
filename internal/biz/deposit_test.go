package biz

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type mockDepositRepo struct {
	profile            *DepositProfile
	existingOrder      *DepositOrder
	existingExemption  *DepositExemptionRecord
	exemptionLockOK    bool
}

func (m *mockDepositRepo) GetOrCreateProfile(context.Context, string, int64) (*DepositProfile, error) {
	if m.profile == nil {
		m.profile = &DepositProfile{UID: "u1001", Status: DepositStatusRequired, DepositAmount: DefaultDepositAmount}
	}
	return m.profile, nil
}
func (m *mockDepositRepo) FindDepositOrderByUIDAndClientReqID(context.Context, string, string) (*DepositOrder, error) {
	return m.existingOrder, nil
}
func (m *mockDepositRepo) CreateDepositOrder(context.Context, *DepositOrder) error { return nil }
func (m *mockDepositRepo) MarkDepositOrderPaying(context.Context, string) error { return nil }
func (m *mockDepositRepo) FindExemptionByUIDAndClientReqID(context.Context, string, string) (*DepositExemptionRecord, error) {
	return m.existingExemption, nil
}
func (m *mockDepositRepo) AcquireExemptionIdempotent(context.Context, string, string, time.Duration) (bool, error) {
	return m.exemptionLockOK, nil
}
func (m *mockDepositRepo) CreateExemptionRecord(_ context.Context, profile *DepositProfile, record *DepositExemptionRecord) error {
	m.existingExemption = record
	if record.Status == ExemptionStatusApproved {
		profile.Exempt = true
		profile.Status = DepositStatusExempt
		profile.ExemptProvider = record.Provider
		profile.ExemptExpireAt = record.ExpireAt
	}
	return nil
}
func (m *mockDepositRepo) ListRecords(context.Context, string, int32, int32) ([]*DepositRecord, error) {
	return nil, nil
}

type mockCreditGateway struct {
	decision *CreditDecision
}

func (m mockCreditGateway) EvaluateExemption(context.Context, string, ExemptionProvider) (*CreditDecision, error) {
	return m.decision, nil
}

func TestApplyExemptionApprovedAllowsBorrow(t *testing.T) {
	repo := &mockDepositRepo{
		profile:         &DepositProfile{UID: "u1001", Status: DepositStatusRequired, DepositAmount: DefaultDepositAmount},
		exemptionLockOK: true,
	}
	uc := NewDepositUsecase(repo, &mockPaymentRepo{}, &mockWxGateway{}, &mockAliGatewayForPayment{}, mockCreditGateway{decision: &CreditDecision{
		Provider:    ExemptionProviderAlipayCredit,
		CreditScore: 700,
		Approved:    true,
		Reason:      "approved",
		ExpireAt:    time.Now().Add(24 * time.Hour),
	}}, log.NewStdLogger(io.Discard))

	record, err := uc.ApplyExemption(context.Background(), ApplyDepositExemptionInput{
		UID:         "u1001",
		Provider:    ExemptionProviderAlipayCredit,
		ClientReqID: "req_1",
	})
	if err != nil {
		t.Fatalf("ApplyExemption() err=%v", err)
	}
	if record.Status != ExemptionStatusApproved {
		t.Fatalf("ApplyExemption() status=%d, want approved", record.Status)
	}
	access, err := uc.HasDepositPrivilege(context.Background(), "u1001")
	if err != nil {
		t.Fatalf("HasDepositPrivilege() err=%v", err)
	}
	if !access.Allowed {
		t.Fatal("HasDepositPrivilege() allowed=false, want true")
	}
}

func TestCreateDepositOrderDuplicateReturnsExisting(t *testing.T) {
	repo := &mockDepositRepo{
		profile: &DepositProfile{UID: "u1001", Status: DepositStatusRequired, DepositAmount: DefaultDepositAmount},
		existingOrder: &DepositOrder{
			DepositOrderNo: "dp_1",
			OutTradeNo:     "otn_1",
			Status:         PayStatusPaying,
			Amount:         DefaultDepositAmount,
		},
	}
	uc := NewDepositUsecase(repo, &mockPaymentRepo{createLockOK: false}, &mockWxGateway{}, &mockAliGatewayForPayment{}, mockCreditGateway{}, log.NewStdLogger(io.Discard))
	order, err := uc.CreateDepositOrder(context.Background(), CreateDepositOrderInput{
		UID:         "u1001",
		Channel:     PayChannelWx,
		PayMode:     PayModeNative,
		ClientReqID: "req_dup",
	})
	if err != nil {
		t.Fatalf("CreateDepositOrder() err=%v", err)
	}
	if order.DepositOrderNo != "dp_1" {
		t.Fatalf("CreateDepositOrder() order_no=%s, want dp_1", order.DepositOrderNo)
	}
}

func TestCreateDepositOrderBlockedByExistingExemption(t *testing.T) {
	repo := &mockDepositRepo{profile: &DepositProfile{
		UID:            "u1001",
		Status:         DepositStatusExempt,
		DepositAmount:  DefaultDepositAmount,
		Exempt:         true,
		ExemptExpireAt: time.Now().Add(time.Hour),
	}}
	uc := NewDepositUsecase(repo, &mockPaymentRepo{createLockOK: true}, &mockWxGateway{}, &mockAliGatewayForPayment{}, mockCreditGateway{}, log.NewStdLogger(io.Discard))
	_, err := uc.CreateDepositOrder(context.Background(), CreateDepositOrderInput{
		UID:         "u1001",
		Channel:     PayChannelWx,
		PayMode:     PayModeNative,
		ClientReqID: "req_1",
	})
	if err == nil {
		t.Fatal("CreateDepositOrder() err=nil, want non-nil")
	}
}
