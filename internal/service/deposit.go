package service

import (
	"context"

	depositv1 "github.com/go-kratos/kratos-layout/api/deposit/v1"
	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/pkg"
)

type DepositService struct {
	depositv1.UnimplementedDepositServiceServer

	uc *biz.DepositUsecase
}

func NewDepositService(uc *biz.DepositUsecase) *DepositService {
	return &DepositService{uc: uc}
}

func (s *DepositService) GetDepositStatus(ctx context.Context, _ *depositv1.GetDepositStatusRequest) (*depositv1.GetDepositStatusReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := s.uc.GetStatus(ctx, claims.UID)
	if err != nil {
		return nil, err
	}
	return &depositv1.GetDepositStatusReply{Profile: toDepositProfile(profile)}, nil
}

func (s *DepositService) CreateDepositOrder(ctx context.Context, req *depositv1.CreateDepositOrderRequest) (*depositv1.CreateDepositOrderReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.CreateDepositOrder(ctx, biz.CreateDepositOrderInput{
		UID:         claims.UID,
		Channel:     toBizDepositChannel(req.GetChannel()),
		PayMode:     biz.PayMode(req.GetPayMode()),
		ClientReqID: req.GetClientReqId(),
		OpenID:      req.GetOpenid(),
	})
	if err != nil {
		return nil, err
	}
	return &depositv1.CreateDepositOrderReply{
		DepositOrderNo: order.DepositOrderNo,
		OutTradeNo:     order.OutTradeNo,
		Status:         depositv1.DepositOrderStatus(order.Status),
		Amount:         order.Amount,
		CodeUrl:        order.CodeURL,
		PayUrl:         order.PayURL,
		JsapiParams:    order.JSAPIParams,
	}, nil
}

func (s *DepositService) ApplyDepositExemption(ctx context.Context, req *depositv1.ApplyDepositExemptionRequest) (*depositv1.ApplyDepositExemptionReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.uc.ApplyExemption(ctx, biz.ApplyDepositExemptionInput{
		UID:         claims.UID,
		Provider:    toBizExemptionProvider(req.GetProvider()),
		ClientReqID: req.GetClientReqId(),
	})
	if err != nil {
		return nil, err
	}
	return &depositv1.ApplyDepositExemptionReply{
		Approved:    record.Status == biz.ExemptionStatusApproved,
		ExemptionId: record.ExemptionID,
		Status:      depositv1.ExemptionStatus(record.Status),
		Provider:    toProtoExemptionProvider(record.Provider),
		CreditScore: record.CreditScore,
		Reason:      record.Reason,
		ExpireAt:    record.ExpireAt.Unix(),
	}, nil
}

func (s *DepositService) ListDepositRecords(ctx context.Context, req *depositv1.ListDepositRecordsRequest) (*depositv1.ListDepositRecordsReply, error) {
	claims, err := pkg.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.uc.ListRecords(ctx, claims.UID, req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	out := make([]*depositv1.DepositRecord, 0, len(items))
	for _, item := range items {
		record := &depositv1.DepositRecord{
			RecordId:          item.RecordID,
			DepositOrderNo:    item.DepositOrderNo,
			OutTradeNo:        item.OutTradeNo,
			OrderStatus:       depositv1.DepositOrderStatus(item.OrderStatus),
			Amount:            item.Amount,
			Channel:           toProtoDepositChannel(item.Channel),
			ExemptionStatus:   depositv1.ExemptionStatus(item.ExemptionStatus),
			ExemptionProvider: toProtoExemptionProvider(item.ExemptionProvider),
			CreditScore:       item.CreditScore,
			Reason:            item.Reason,
			CreatedAt:         item.CreatedAt.Unix(),
			ExpireAt:          item.ExpireAt.Unix(),
		}
		if item.RecordType == "ORDER" {
			record.RecordType = depositv1.DepositRecordType_DEPOSIT_RECORD_TYPE_ORDER
		} else {
			record.RecordType = depositv1.DepositRecordType_DEPOSIT_RECORD_TYPE_EXEMPTION
		}
		out = append(out, record)
	}
	return &depositv1.ListDepositRecordsReply{Items: out}, nil
}

func toDepositProfile(profile *biz.DepositProfile) *depositv1.DepositProfile {
	if profile == nil {
		return nil
	}
	return &depositv1.DepositProfile{
		Uid:            profile.UID,
		Status:         depositv1.DepositStatus(profile.Status),
		DepositAmount:  profile.DepositAmount,
		Paid:           profile.Paid,
		Exempt:         profile.Exempt,
		ActiveOrderNo:  profile.ActiveOrderNo,
		ExemptProvider: toProtoExemptionProvider(profile.ExemptProvider),
		ExemptExpireAt: profile.ExemptExpireAt.Unix(),
	}
}

func toBizDepositChannel(channel depositv1.DepositChannel) string {
	switch channel {
	case depositv1.DepositChannel_DEPOSIT_CHANNEL_WECHAT:
		return biz.PayChannelWx
	case depositv1.DepositChannel_DEPOSIT_CHANNEL_ALIPAY:
		return biz.PayChannelAl
	default:
		return ""
	}
}

func toProtoDepositChannel(channel string) depositv1.DepositChannel {
	switch channel {
	case biz.PayChannelWx:
		return depositv1.DepositChannel_DEPOSIT_CHANNEL_WECHAT
	case biz.PayChannelAl:
		return depositv1.DepositChannel_DEPOSIT_CHANNEL_ALIPAY
	default:
		return depositv1.DepositChannel_DEPOSIT_CHANNEL_UNSPECIFIED
	}
}

func toBizExemptionProvider(provider depositv1.ExemptionProvider) biz.ExemptionProvider {
	switch provider {
	case depositv1.ExemptionProvider_EXEMPTION_PROVIDER_ALIPAY_CREDIT:
		return biz.ExemptionProviderAlipayCredit
	case depositv1.ExemptionProvider_EXEMPTION_PROVIDER_WECHAT_CREDIT:
		return biz.ExemptionProviderWechatCredit
	default:
		return ""
	}
}

func toProtoExemptionProvider(provider biz.ExemptionProvider) depositv1.ExemptionProvider {
	switch provider {
	case biz.ExemptionProviderAlipayCredit:
		return depositv1.ExemptionProvider_EXEMPTION_PROVIDER_ALIPAY_CREDIT
	case biz.ExemptionProviderWechatCredit:
		return depositv1.ExemptionProvider_EXEMPTION_PROVIDER_WECHAT_CREDIT
	default:
		return depositv1.ExemptionProvider_EXEMPTION_PROVIDER_UNSPECIFIED
	}
}
