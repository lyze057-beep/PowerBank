package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	chargerv1 "github.com/go-kratos/kratos-layout/api/charger/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

const BorrowIdempotentTTL = 120 * time.Second

var (
	ErrChargerInvalidArgument   = errors.BadRequest(chargerv1.ErrorReason_INVALID_ARGUMENT.String(), "invalid charger argument")
	ErrChargerStationNotFound   = errors.NotFound(chargerv1.ErrorReason_CHARGER_STATION_NOT_FOUND.String(), "station not found")
	ErrChargerStationOffline    = errors.BadRequest(chargerv1.ErrorReason_CHARGER_STATION_OFFLINE.String(), "station offline")
	ErrChargerNoAvailableBank   = errors.BadRequest(chargerv1.ErrorReason_CHARGER_NO_AVAILABLE_POWERBANK.String(), "no available powerbank")
	ErrChargerDuplicateRequest  = errors.BadRequest(chargerv1.ErrorReason_CHARGER_DUPLICATE_REQUEST.String(), "duplicate borrow request")
	ErrChargerOrderConflict     = errors.BadRequest(chargerv1.ErrorReason_CHARGER_ORDER_CONFLICT.String(), "charger order conflict")
	ErrChargerDepositRequired   = errors.BadRequest(chargerv1.ErrorReason_CHARGER_DEPOSIT_REQUIRED.String(), "deposit required")
	ErrChargerOrderNotFound     = errors.NotFound(chargerv1.ErrorReason_CHARGER_ORDER_NOT_FOUND.String(), "charger order not found")
)

type ScanBorrowInput struct {
	UID         string
	StationID   string
	ClientReqID string
}

type BorrowCallbackInput struct {
	RentOrderNo string
	Success     bool
	Reason      string
}

type ReturnCallbackInput struct {
	RentOrderNo string
	StationID   string
	SlotID      string
	Success     bool
	Reason      string
}

type ChargerRepo interface {
	AcquireBorrowIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error)
	FindRentOrderByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*RentOrder, error)
	HasActiveOrder(ctx context.Context, uid string) (bool, error)
	CreateBorrowOrder(ctx context.Context, uid, stationID, clientReqID string, depositAmount int64) (*RentOrder, error)
	HandleBorrowResult(ctx context.Context, rentOrderNo string, success bool, reason string) (*RentOrder, error)
	HandleReturnResult(ctx context.Context, rentOrderNo, stationID, slotID string, success bool, reason string) (*RentOrder, error)
}

type ChargerUsecase struct {
	repo    ChargerRepo
	deposit *DepositUsecase
	notify  *NotifyUsecase
	log     *log.Helper
	nowFunc func() time.Time
}

func NewChargerUsecase(repo ChargerRepo, deposit *DepositUsecase, notify *NotifyUsecase, logger log.Logger) *ChargerUsecase {
	return &ChargerUsecase{
		repo:    repo,
		deposit: deposit,
		notify:  notify,
		log:     log.NewHelper(log.With(logger, "module", "biz/charger")),
		nowFunc: time.Now,
	}
}

func (uc *ChargerUsecase) ScanBorrow(ctx context.Context, in ScanBorrowInput) (*RentOrder, error) {
	if strings.TrimSpace(in.UID) == "" || strings.TrimSpace(in.StationID) == "" || strings.TrimSpace(in.ClientReqID) == "" {
		return nil, ErrChargerInvalidArgument
	}
	access, err := uc.deposit.HasDepositPrivilege(ctx, in.UID)
	if err != nil {
		return nil, err
	}
	if !access.Allowed {
		return nil, ErrChargerDepositRequired
	}
	active, err := uc.repo.HasActiveOrder(ctx, in.UID)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, ErrOrderAlreadyActive
	}
	ok, err := uc.repo.AcquireBorrowIdempotent(ctx, in.UID, in.ClientReqID, BorrowIdempotentTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		existing, exErr := uc.repo.FindRentOrderByUIDAndClientReqID(ctx, in.UID, in.ClientReqID)
		if exErr != nil {
			return nil, exErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, ErrChargerDuplicateRequest
	}
	return uc.repo.CreateBorrowOrder(ctx, in.UID, in.StationID, in.ClientReqID, access.DepositAmount)
}

func (uc *ChargerUsecase) HandleBorrowResult(ctx context.Context, in BorrowCallbackInput) (*RentOrder, error) {
	if strings.TrimSpace(in.RentOrderNo) == "" {
		return nil, ErrChargerInvalidArgument
	}
	order, err := uc.repo.HandleBorrowResult(ctx, in.RentOrderNo, in.Success, in.Reason)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrChargerOrderNotFound
	}
	if uc.notify != nil {
		title := "Borrow Pending"
		content := fmt.Sprintf("Order %s is waiting for cabinet confirmation.", order.RentOrderNo)
		if in.Success {
			title = "Borrow Success"
			content = fmt.Sprintf("Powerbank %s has been borrowed successfully.", order.PowerbankID)
		} else if strings.TrimSpace(in.Reason) != "" {
			title = "Borrow Failed"
			content = in.Reason
		}
		_, _ = uc.notify.PushMessage(context.Background(), PushMessageInput{
			UID:         order.UID,
			Title:       title,
			Content:     content,
			BizType:     "RENT_ORDER",
			BizID:       order.RentOrderNo,
			ClientReqID: "notify_borrow_" + order.RentOrderNo,
		})
	}
	return order, nil
}

func (uc *ChargerUsecase) HandleReturnResult(ctx context.Context, in ReturnCallbackInput) (*RentOrder, error) {
	if strings.TrimSpace(in.RentOrderNo) == "" || strings.TrimSpace(in.StationID) == "" || strings.TrimSpace(in.SlotID) == "" {
		return nil, ErrChargerInvalidArgument
	}
	order, err := uc.repo.HandleReturnResult(ctx, in.RentOrderNo, in.StationID, in.SlotID, in.Success, in.Reason)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrChargerOrderNotFound
	}
	if uc.notify != nil {
		title := "Return Processing"
		content := fmt.Sprintf("Return event for order %s is being processed.", order.RentOrderNo)
		if in.Success && order.PayStatus == RentPayStatusUnpaid {
			title = "Return Success"
			content = fmt.Sprintf("Return succeeded. Rent fee %.2f CNY is pending payment.", float64(order.RentFee)/100.0)
		} else if in.Success {
			title = "Return Completed"
			content = "Return succeeded and no additional payment is required."
		} else if strings.TrimSpace(in.Reason) != "" {
			title = "Return Failed"
			content = in.Reason
		}
		_, _ = uc.notify.PushMessage(context.Background(), PushMessageInput{
			UID:         order.UID,
			Title:       title,
			Content:     content,
			BizType:     "RENT_ORDER",
			BizID:       order.RentOrderNo,
			ClientReqID: "notify_return_" + order.RentOrderNo,
		})
	}
	return order, nil
}

func newRentOrderNo(now time.Time) string {
	return "RO" + now.Format("20060102150405") + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:12]
}

// NewRentOrderNoForData keeps rent-order numbering consistent between biz and data layers.
func NewRentOrderNoForData(now time.Time) string {
	return newRentOrderNo(now)
}
