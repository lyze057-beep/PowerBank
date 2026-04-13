package service

import (
	"context"

	chargerv1 "github.com/go-kratos/kratos-layout/api/charger/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

type ChargerService struct {
	chargerv1.UnimplementedChargerServiceServer

	uc *biz.ChargerUsecase
}

func NewChargerService(uc *biz.ChargerUsecase) *ChargerService {
	return &ChargerService{uc: uc}
}

func (s *ChargerService) ScanBorrow(ctx context.Context, req *chargerv1.ScanBorrowRequest) (*chargerv1.ScanBorrowReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.ScanBorrow(ctx, biz.ScanBorrowInput{
		UID:         claims.UID,
		StationID:   req.GetStationId(),
		ClientReqID: req.GetClientReqId(),
	})
	if err != nil {
		return nil, err
	}
	return &chargerv1.ScanBorrowReply{
		RentOrderNo: order.RentOrderNo,
		PowerbankId: order.PowerbankID,
		SlotId:      order.BorrowSlotID,
		Status:      chargerv1.BorrowStatus_BORROW_STATUS_PENDING,
	}, nil
}

func (s *ChargerService) NotifyBorrowResult(ctx context.Context, req *chargerv1.NotifyBorrowResultRequest) (*chargerv1.NotifyBorrowResultReply, error) {
	_, err := s.uc.HandleBorrowResult(ctx, biz.BorrowCallbackInput{
		RentOrderNo: req.GetRentOrderNo(),
		Success:     req.GetSuccess(),
		Reason:      req.GetReason(),
	})
	if err != nil {
		return nil, err
	}
	return &chargerv1.NotifyBorrowResultReply{Success: true}, nil
}

func (s *ChargerService) NotifyReturnResult(ctx context.Context, req *chargerv1.NotifyReturnResultRequest) (*chargerv1.NotifyReturnResultReply, error) {
	_, err := s.uc.HandleReturnResult(ctx, biz.ReturnCallbackInput{
		RentOrderNo: req.GetRentOrderNo(),
		StationID:   req.GetStationId(),
		SlotID:      req.GetSlotId(),
		Success:     req.GetSuccess(),
		Reason:      req.GetReason(),
	})
	if err != nil {
		return nil, err
	}
	return &chargerv1.NotifyReturnResultReply{Success: true}, nil
}
