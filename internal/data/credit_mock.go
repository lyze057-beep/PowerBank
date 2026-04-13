package data

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

type creditGateway struct {
	cfg *conf.Credit
	log *log.Helper
}

func NewCreditGateway(c *conf.Credit, logger log.Logger) biz.CreditGateway {
	if c == nil {
		c = &conf.Credit{
			Enabled:       true,
			DefaultScore:  680,
			ApprovalScore: 650,
			ExemptDays:    30,
		}
	}
	return &creditGateway{
		cfg: c,
		log: log.NewHelper(log.With(logger, "module", "data/credit")),
	}
}

func (g *creditGateway) EvaluateExemption(_ context.Context, uid string, provider biz.ExemptionProvider) (*biz.CreditDecision, error) {
	score := g.cfg.DefaultScore + scoreOffset(uid)
	decision := &biz.CreditDecision{
		Provider:    provider,
		CreditScore: score,
		Approved:    g.cfg.Enabled && score >= g.cfg.ApprovalScore,
		ExpireAt:    time.Now().Add(time.Duration(g.cfg.ExemptDays) * 24 * time.Hour),
	}
	if decision.Approved {
		decision.Reason = "credit approved"
	} else {
		decision.Reason = "credit score below threshold"
	}
	return decision, nil
}

func scoreOffset(uid string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(uid))
	return int32(h.Sum32()%61) - 30
}
