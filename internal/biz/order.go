package biz

import (
	"context"
	"strings"
	"time"

	orderv1 "github.com/go-kratos/kratos-layout/api/order/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

var (
	ErrOrderInvalidArgument = errors.BadRequest(orderv1.ErrorReason_INVALID_ARGUMENT.String(), "invalid order argument")
	ErrOrderNotFound        = errors.NotFound(orderv1.ErrorReason_ORDER_NOT_FOUND.String(), "order not found")
	ErrOrderAlreadyActive   = errors.BadRequest(orderv1.ErrorReason_ORDER_ALREADY_ACTIVE.String(), "order already active")
	ErrOrderDuplicate       = errors.BadRequest(orderv1.ErrorReason_ORDER_DUPLICATE_REQUEST.String(), "order duplicate request")
	ErrOrderReturnConflict  = errors.BadRequest(orderv1.ErrorReason_ORDER_RETURN_CONFLICT.String(), "order return conflict")
)

type RentOrderStatus int32

const (
	RentOrderStatusPendingBorrow RentOrderStatus = 1
	RentOrderStatusInUse         RentOrderStatus = 2
	RentOrderStatusReturnPending RentOrderStatus = 3
	RentOrderStatusPayPending    RentOrderStatus = 4
	RentOrderStatusCompleted     RentOrderStatus = 5
	RentOrderStatusCancelled     RentOrderStatus = 6
	RentOrderStatusException     RentOrderStatus = 7
)

type RentPayStatus int32

const (
	RentPayStatusNotRequired RentPayStatus = 1
	RentPayStatusUnpaid      RentPayStatus = 2
	RentPayStatusPaid        RentPayStatus = 3
)

type OrderExceptionType string

const (
	OrderExceptionTypeReturnFailure OrderExceptionType = "RETURN_FAILURE"
	OrderExceptionTypeDeviceDamage  OrderExceptionType = "DEVICE_DAMAGE"
	OrderExceptionTypeWrongFee      OrderExceptionType = "WRONG_FEE"
	OrderExceptionTypeOther         OrderExceptionType = "OTHER"
)

type RentOrder struct {
	UID               string
	RentOrderNo       string
	ClientReqID       string
	StationID         string
	ReturnStationID   string
	PowerbankID       string
	BorrowSlotID      string
	ReturnSlotID      string
	PricingRuleID     string
	PricingRuleName   string
	Status            RentOrderStatus
	PayStatus         RentPayStatus
	DepositAmount     int64
	RentFee           int64
	PaymentOutTradeNo string
	BorrowedAt        time.Time
	ReturnedAt        time.Time
	CreatedAt         time.Time
	ExceptionReported bool
	ExceptionDesc     string
}

type ReportOrderExceptionInput struct {
	UID         string
	RentOrderNo string
	ClientReqID string
	Type        OrderExceptionType
	Description string
}

type OrderRepo interface {
	GetCurrentOrder(ctx context.Context, uid string) (*RentOrder, error)
	ListOrders(ctx context.Context, uid string, page, pageSize int32) ([]*RentOrder, error)
	GetOrderDetail(ctx context.Context, uid, rentOrderNo string) (*RentOrder, error)
	ReportException(ctx context.Context, in ReportOrderExceptionInput) error
}

type OrderUsecase struct {
	repo OrderRepo
	log  *log.Helper
}

func NewOrderUsecase(repo OrderRepo, logger log.Logger) *OrderUsecase {
	return &OrderUsecase{repo: repo, log: log.NewHelper(log.With(logger, "module", "biz/order"))}
}

func (uc *OrderUsecase) GetCurrentOrder(ctx context.Context, uid string) (*RentOrder, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, ErrOrderInvalidArgument
	}
	return uc.repo.GetCurrentOrder(ctx, uid)
}

func (uc *OrderUsecase) ListOrders(ctx context.Context, uid string, page, pageSize int32) ([]*RentOrder, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, ErrOrderInvalidArgument
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}
	return uc.repo.ListOrders(ctx, uid, page, pageSize)
}

func (uc *OrderUsecase) GetOrderDetail(ctx context.Context, uid, rentOrderNo string) (*RentOrder, error) {
	if strings.TrimSpace(uid) == "" || strings.TrimSpace(rentOrderNo) == "" {
		return nil, ErrOrderInvalidArgument
	}
	order, err := uc.repo.GetOrderDetail(ctx, uid, rentOrderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrOrderNotFound
	}
	return order, nil
}

func (uc *OrderUsecase) ReportException(ctx context.Context, in ReportOrderExceptionInput) error {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.RentOrderNo) == "" || strings.TrimSpace(in.ClientReqID) == "" || strings.TrimSpace(in.Description) == "" {
		return ErrOrderInvalidArgument
	}
	if strings.TrimSpace(string(in.Type)) == "" {
		return ErrOrderInvalidArgument
	}
	return uc.repo.ReportException(ctx, in)
}
