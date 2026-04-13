package service

import (
	"context"

	orderv1 "github.com/go-kratos/kratos-layout/api/order/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

type OrderService struct {
	orderv1.UnimplementedOrderServiceServer

	uc *biz.OrderUsecase
}

func NewOrderService(uc *biz.OrderUsecase) *OrderService {
	return &OrderService{uc: uc}
}

func (s *OrderService) GetCurrentOrder(ctx context.Context, _ *orderv1.GetCurrentOrderRequest) (*orderv1.GetCurrentOrderReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.GetCurrentOrder(ctx, claims.UID)
	if err != nil {
		return nil, err
	}
	return &orderv1.GetCurrentOrderReply{Order: toOrderDetail(order)}, nil
}

func (s *OrderService) ListOrders(ctx context.Context, req *orderv1.ListOrdersRequest) (*orderv1.ListOrdersReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.uc.ListOrders(ctx, claims.UID, req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	out := make([]*orderv1.OrderSummary, 0, len(items))
	for _, item := range items {
		out = append(out, &orderv1.OrderSummary{
			RentOrderNo: item.RentOrderNo,
			StationId:   item.StationID,
			PowerbankId: item.PowerbankID,
			Status:      orderv1.RentOrderStatus(item.Status),
			PayStatus:   orderv1.RentPayStatus(item.PayStatus),
			RentFee:     item.RentFee,
			CreatedAt:   item.CreatedAt.Unix(),
			BorrowAt:    item.BorrowedAt.Unix(),
			ReturnAt:    item.ReturnedAt.Unix(),
		})
	}
	return &orderv1.ListOrdersReply{Items: out}, nil
}

func (s *OrderService) GetOrderDetail(ctx context.Context, req *orderv1.GetOrderDetailRequest) (*orderv1.GetOrderDetailReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.GetOrderDetail(ctx, claims.UID, req.GetRentOrderNo())
	if err != nil {
		return nil, err
	}
	return &orderv1.GetOrderDetailReply{Order: toOrderDetail(order)}, nil
}

func (s *OrderService) ReportOrderException(ctx context.Context, req *orderv1.ReportOrderExceptionRequest) (*orderv1.ReportOrderExceptionReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.uc.ReportException(ctx, biz.ReportOrderExceptionInput{
		UID:         claims.UID,
		RentOrderNo: req.GetRentOrderNo(),
		ClientReqID: req.GetClientReqId(),
		Type:        toBizExceptionType(req.GetExceptionType()),
		Description: req.GetDescription(),
	}); err != nil {
		return nil, err
	}
	return &orderv1.ReportOrderExceptionReply{Success: true}, nil
}

func toOrderDetail(order *biz.RentOrder) *orderv1.OrderDetail {
	if order == nil {
		return nil
	}
	return &orderv1.OrderDetail{
		RentOrderNo:      order.RentOrderNo,
		StationId:        order.StationID,
		ReturnStationId:  order.ReturnStationID,
		PowerbankId:      order.PowerbankID,
		BorrowSlotId:     order.BorrowSlotID,
		ReturnSlotId:     order.ReturnSlotID,
		Status:           orderv1.RentOrderStatus(order.Status),
		PayStatus:        orderv1.RentPayStatus(order.PayStatus),
		DepositAmount:    order.DepositAmount,
		RentFee:          order.RentFee,
		BorrowAt:         order.BorrowedAt.Unix(),
		ReturnAt:         order.ReturnedAt.Unix(),
		CreatedAt:        order.CreatedAt.Unix(),
		CurrentOutTradeNo: order.PaymentOutTradeNo,
		PricingRuleName:  order.PricingRuleName,
		ExceptionReported: order.ExceptionReported,
		ExceptionDesc:    order.ExceptionDesc,
	}
}

func toBizExceptionType(t orderv1.ExceptionType) biz.OrderExceptionType {
	switch t {
	case orderv1.ExceptionType_EXCEPTION_TYPE_RETURN_FAILURE:
		return biz.OrderExceptionTypeReturnFailure
	case orderv1.ExceptionType_EXCEPTION_TYPE_DEVICE_DAMAGE:
		return biz.OrderExceptionTypeDeviceDamage
	case orderv1.ExceptionType_EXCEPTION_TYPE_WRONG_FEE:
		return biz.OrderExceptionTypeWrongFee
	default:
		return biz.OrderExceptionTypeOther
	}
}
