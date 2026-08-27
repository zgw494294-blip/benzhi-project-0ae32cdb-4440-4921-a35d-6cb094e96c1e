package workflow

import (
	"benzhiguji/internal/assessment"
	"benzhiguji/internal/domain"
	"fmt"
	"strings"
	"time"
)

type ActionResult struct {
	Case    domain.RestorationCase
	Message string
	Changed bool
}

func (s *Service) AddRegionChecked(role string, x domain.DamageRegion, expected int) (ActionResult, error) {
	if e := RequireRole(role, "region"); e != nil {
		return ActionResult{}, e
	}
	v, e := s.AddRegion(x, expected, "")
	if e != nil {
		return ActionResult{}, e
	}
	c, _ := s.Repo.GetCase(x.CaseID)
	return ActionResult{c, "病害区域 " + v.RegionCode + " 已登记", true}, nil
}
func (s *Service) AddPlanChecked(role string, p domain.TreatmentPlanRevision, expected int) (ActionResult, error) {
	if e := RequireRole(role, "plan"); e != nil {
		return ActionResult{}, e
	}
	v, e := s.AddPlan(p, expected, "")
	if e != nil {
		return ActionResult{}, e
	}
	c, _ := s.Repo.GetCase(p.CaseID)
	return ActionResult{c, fmt.Sprintf("方案修订 %d 已保存", v.RevisionNo), true}, nil
}
func (s *Service) EvaluateSummary(id string, expected int) (ActionResult, error) {
	a, e := s.Evaluate(id, expected)
	if e != nil {
		return ActionResult{}, e
	}
	c, _ := s.Repo.GetCase(id)
	msg := "评估通过"
	if assessment.HasBlockingOpen(a) {
		msg = "发现阻断风险"
	}
	return ActionResult{c, msg, true}, nil
}
func (s *Service) RequestRemediation(id string, expected int, evidence string) (ActionResult, error) {
	if e := s.CloseWithEvidence(id, expected, evidence); e != nil {
		return ActionResult{}, e
	}
	c, _ := s.Repo.GetCase(id)
	return ActionResult{c, "整改证据已提交，等待复验", true}, nil
}
func (s *Service) RejectReview(id string, expected int, reviewer, reason string) (ActionResult, error) {
	if strings.TrimSpace(reason) == "" {
		return ActionResult{}, domain.ErrInvalid
	}
	_, e := s.Decide(id, expected, reviewer, "reject", reason)
	if e != nil {
		return ActionResult{}, e
	}
	c, _ := s.Repo.GetCase(id)
	return ActionResult{c, "复核退回：" + reason, true}, nil
}
func (s *Service) ApproveReview(id string, expected int, reviewer string) (domain.WorkPermit, error) {
	return s.Decide(id, expected, reviewer, "approve", "")
}
func (s *Service) PermitDigest(id string) (string, error) { return s.PermitManifest(id) }
func (s *Service) Freshness(id string, maxAge time.Duration) bool {
	p, e := s.Repo.Permit(id)
	return e == nil && time.Since(p.IssuedAt) <= maxAge
}
