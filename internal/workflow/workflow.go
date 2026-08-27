package workflow

import (
	"benzhiguji/internal/assessment"
	"benzhiguji/internal/domain"
	"benzhiguji/internal/store"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Service struct {
	Repo             *store.Repository
	Evaluator        assessment.Evaluator
	candidateMu      sync.Mutex
	candidateDigests map[string]string
}

func New(r *store.Repository) *Service {
	return &Service{
		Repo:             r,
		Evaluator:        assessment.DefaultEvaluator(),
		candidateDigests: map[string]string{},
	}
}
func (s *Service) CreateCase(c domain.RestorationCase, key string) (domain.RestorationCase, error) {
	c.ID = nonempty(c.ID, fmt.Sprintf("case-%d", time.Now().UnixNano()))
	c.Status = domain.StatusDraft
	if e := domain.ValidateCase(c); e != nil {
		return c, e
	}
	return s.Repo.CreateCase(c, key)
}
func (s *Service) AddRegion(x domain.DamageRegion, expected int, key string) (domain.DamageRegion, error) {
	return s.AddRegions([]domain.DamageRegion{x}, expected, key)
}
func (s *Service) AddRegions(xs []domain.DamageRegion, expected int, key string) (domain.DamageRegion, error) {
	if expected < 1 {
		return domain.DamageRegion{}, fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	if len(xs) == 0 {
		return domain.DamageRegion{}, domain.ErrInvalid
	}
	x := xs[0]
	c, e := s.Repo.GetCase(x.CaseID)
	if e != nil {
		return x, e
	}
	if c.Status == domain.StatusFrozen {
		return x, domain.ErrFrozen
	}
	if c.Status == domain.StatusReview || c.Status == domain.StatusApproved {
		return x, domain.ErrTransition
	}
	if c.Version != expected {
		return x, domain.ErrConflict
	}
	seen := map[string]bool{}
	seenIDs := map[string]bool{}
	var validation []string
	for i := range xs {
		if xs[i].CaseID != "" && xs[i].CaseID != x.CaseID {
			validation = append(validation, fmt.Sprintf("[%d] 案卷标识不一致", i))
		}
		xs[i].CaseID = x.CaseID
		if e = domain.ValidateRegion(xs[i]); e != nil {
			validation = append(validation, fmt.Sprintf("[%d] %v", i, e))
		}
		if seen[xs[i].RegionCode] {
			validation = append(validation, fmt.Sprintf("[%d] 区域编号 %s 重复", i, xs[i].RegionCode))
		}
		seen[xs[i].RegionCode] = true
		if xs[i].ID != "" {
			if seenIDs[xs[i].ID] {
				validation = append(validation, fmt.Sprintf("[%d] 区域标识重复", i))
			}
			seenIDs[xs[i].ID] = true
		}
	}
	rs, _ := s.Repo.Regions(x.CaseID)
	for _, r := range rs {
		if seen[r.RegionCode] {
			validation = append(validation, fmt.Sprintf("区域 %s 编号重复", r.RegionCode))
		}
		if seenIDs[r.ID] {
			validation = append(validation, fmt.Sprintf("区域 %s 标识重复", r.RegionCode))
		}
	}
	if len(validation) > 0 {
		return x, fmt.Errorf("%w: %s", domain.ErrInvalid, strings.Join(validation, "；"))
	}
	for i := range xs {
		xs[i].ID = nonempty(xs[i].ID, fmt.Sprintf("region-%d-%d", time.Now().UnixNano(), i))
		if xs[i].Revision == 0 {
			xs[i].Revision = 1
		}
	}
	if _, e = s.Repo.AddRegionsAndBump(x.CaseID, xs, expected, key); e != nil {
		return x, e
	}
	return xs[0], nil
}
func (s *Service) AddPlan(p domain.TreatmentPlanRevision, expected int, key string) (domain.TreatmentPlanRevision, error) {
	if expected < 1 {
		return p, fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	c, e := s.Repo.GetCase(p.CaseID)
	if e != nil {
		return p, e
	}
	if c.Status == domain.StatusFrozen {
		return p, domain.ErrFrozen
	}
	if c.Status != domain.StatusDraft && c.Status != domain.StatusPlanning && c.Status != domain.StatusRemediation && c.Status != domain.StatusEvaluated {
		return p, domain.ErrTransition
	}
	if c.Version != expected {
		return p, domain.ErrConflict
	}
	rs, _ := s.Repo.Regions(p.CaseID)
	if len(rs) == 0 {
		return p, fmt.Errorf("%w: 至少登记一个病害区域", domain.ErrInvalid)
	}
	if e = domain.ValidatePlan(p, rs); e != nil {
		return p, e
	}
	p.ID = nonempty(p.ID, fmt.Sprintf("plan-%d", time.Now().UnixNano()))
	p.CaseID = c.ID
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	return s.Repo.AddPlanAndBump(p, expected)
}
func (s *Service) AddCoupon(x domain.TrialCouponRevision, expected int) (domain.TrialCouponRevision, error) {
	if expected < 1 {
		return x, fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	c, e := s.Repo.GetCase(x.CaseID)
	if e != nil {
		return x, e
	}
	if c.Status == domain.StatusFrozen {
		return x, domain.ErrFrozen
	}
	if c.Status == domain.StatusReview || c.Status == domain.StatusApproved {
		return x, domain.ErrTransition
	}
	if c.Version != expected {
		return x, domain.ErrConflict
	}
	if e = domain.ValidateCoupon(x); e != nil {
		return x, e
	}
	p, _ := s.Repo.LatestPlan(x.CaseID)
	if p.ID == "" {
		return x, fmt.Errorf("%w: 试片必须关联方案", domain.ErrInvalid)
	}
	if x.PlanRevisionID == "" {
		x.PlanRevisionID = p.ID
	}
	if x.PlanRevisionID != p.ID {
		return x, fmt.Errorf("%w: 试片不属于当前方案", domain.ErrInvalid)
	}
	x.ID = nonempty(x.ID, fmt.Sprintf("coupon-%d", time.Now().UnixNano()))
	if x.RecordedAt.IsZero() {
		x.RecordedAt = time.Now()
	}
	return s.Repo.AddCouponAndBump(x, expected)
}
func (s *Service) Evaluate(caseID string, expected int) (domain.AssessmentSnapshot, error) {
	if expected < 1 {
		return domain.AssessmentSnapshot{}, fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	if s.Evaluator.MaxColorDelta <= 0 || s.Evaluator.MinPH <= 0 || s.Evaluator.MinPeel <= 0 || s.Evaluator.MinObservation <= 0 {
		s.Evaluator = assessment.DefaultEvaluator()
	}
	c, e := s.Repo.GetCase(caseID)
	if e != nil {
		return domain.AssessmentSnapshot{}, e
	}
	if c.Version != expected {
		return domain.AssessmentSnapshot{}, domain.ErrConflict
	}
	if c.Status == domain.StatusFrozen {
		return domain.AssessmentSnapshot{}, domain.ErrFrozen
	}
	if c.Status == domain.StatusReview || c.Status == domain.StatusApproved {
		return domain.AssessmentSnapshot{}, domain.ErrTransition
	}
	p, e := s.Repo.LatestPlan(caseID)
	if e != nil {
		return domain.AssessmentSnapshot{}, e
	}
	rs, _ := s.Repo.Regions(caseID)
	if e := domain.EnsurePlanCoverage(p, rs); e != nil {
		return domain.AssessmentSnapshot{}, e
	}
	cs, _ := s.Repo.Coupons(caseID)
	latest := map[string]domain.TrialCouponRevision{}
	for _, x := range cs {
		if old, ok := latest[x.CouponCode]; !ok || x.RevisionNo > old.RevisionNo {
			latest[x.CouponCode] = x
		}
	}
	for _, x := range latest {
		if x.PlanRevisionID == "" || x.PlanRevisionID != p.ID {
			return domain.AssessmentSnapshot{}, fmt.Errorf("%w: 试片 %s 与最新方案不匹配", domain.ErrInvalid, x.CouponCode)
		}
		if e := domain.ValidateCoupon(x); e != nil {
			return domain.AssessmentSnapshot{}, e
		}
	}
	a := s.Evaluator.Evaluate(c, p, rs, cs)
	if _, e = s.Repo.SaveAssessment(a); e != nil {
		return a, e
	}
	if a.Findings == nil {
		a.Findings = []domain.RiskFinding{}
	}
	if assessment.HasBlockingOpen(a) {
		c.Status = domain.StatusRemediation
	} else {
		c.Status = domain.StatusEvaluated
	}
	_, e = s.Repo.UpdateCase(c, expected, "")
	return a, e
}
func (s *Service) Resolve(caseID string, expected int, evidence string) error {
	if expected < 1 {
		return fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	if evidence == "" {
		return domain.ErrInvalid
	}
	c0, e0 := s.Repo.GetCase(caseID)
	if e0 != nil {
		return e0
	}
	if c0.Version != expected {
		return domain.ErrConflict
	}
	if c0.Status == domain.StatusFrozen {
		return domain.ErrFrozen
	}
	if e := s.Repo.CloseRisks(caseID, evidence); e != nil {
		return e
	}
	c, e := s.Repo.GetCase(caseID)
	if e != nil {
		return e
	}
	c.Status = domain.StatusRemediation
	_, e = s.Repo.UpdateCase(c, expected, "")
	return e
}
func (s *Service) ResolveRisk(caseID string, expected int, riskID, evidence string) error {
	if expected < 1 {
		return fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	if len([]rune(strings.TrimSpace(evidence))) < 4 {
		return domain.ErrInvalid
	}
	c, e := s.Repo.GetCase(caseID)
	if e != nil {
		return e
	}
	if c.Version != expected {
		return domain.ErrConflict
	}
	if c.Status == domain.StatusFrozen {
		return domain.ErrFrozen
	}
	if e = s.Repo.CloseRisk(caseID, riskID, evidence); e != nil {
		return e
	}
	c.Status = domain.StatusRemediation
	_, e = s.Repo.UpdateCase(c, expected, "")
	return e
}
func (s *Service) SubmitReview(caseID string, expected int, reviewer string) (domain.RestorationCase, error) {
	if expected < 1 {
		return domain.RestorationCase{}, fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	c, e := s.Repo.GetCase(caseID)
	if e != nil {
		return c, e
	}
	if c.Version != expected {
		return c, domain.ErrConflict
	}
	a, e := s.Repo.LatestAssessment(caseID)
	if e != nil {
		return c, e
	}
	if assessment.HasBlockingOpen(a) {
		return c, fmt.Errorf("%w: 仍有阻断风险", domain.ErrInvalid)
	}
	if c.Status != domain.StatusEvaluated && c.Status != domain.StatusRemediation {
		return c, domain.ErrTransition
	}
	c.Status = domain.StatusReview
	_, e = s.Repo.UpdateCase(c, expected, "")
	return c, e
}
func (s *Service) Decide(caseID string, expected int, reviewer, decision, reason string) (domain.WorkPermit, error) {
	if expected < 1 {
		return domain.WorkPermit{}, fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	if strings.TrimSpace(reviewer) == "" {
		return domain.WorkPermit{}, fmt.Errorf("%w: 复核员不能为空", domain.ErrInvalid)
	}
	c, e := s.Repo.GetCase(caseID)
	if e != nil {
		return domain.WorkPermit{}, e
	}
	if c.Version != expected {
		return domain.WorkPermit{}, domain.ErrConflict
	}
	a, e := s.Repo.LatestAssessment(caseID)
	if e != nil {
		return domain.WorkPermit{}, e
	}
	bundle, _ := s.Repo.Bundle(caseID)
	digest := domain.ManifestDigest(struct {
		C domain.RestorationCase
		R []domain.DamageRegion
		P domain.TreatmentPlanRevision
		T []domain.TrialCouponRevision
		A domain.AssessmentSnapshot
	}{c, bundle.Regions, bundle.Plan, bundle.Coupons, a})
	d := domain.ReviewDecision{ID: fmt.Sprintf("review-%d", time.Now().UnixNano()), CaseID: caseID, CandidateDigest: digest, Reviewer: reviewer, Decision: decision, Reason: reason, CreatedAt: time.Now()}
	if c.Status != domain.StatusReview {
		return domain.WorkPermit{}, domain.ErrTransition
	}
	if decision == "reject" {
		if reason == "" {
			return domain.WorkPermit{}, domain.ErrInvalid
		}
		d.Decision = "reject"
		if e = s.Repo.SaveReview(d); e != nil {
			return domain.WorkPermit{}, e
		}
		c.Status = domain.StatusRemediation
		_, e = s.Repo.UpdateCase(c, expected, "")
		return domain.WorkPermit{}, e
	}
	if decision != "approve" {
		return domain.WorkPermit{}, domain.ErrTransition
	}
	if e = s.Repo.SaveReview(d); e != nil {
		return domain.WorkPermit{}, e
	}
	c.Status = domain.StatusApproved
	frozenVersion := c.Version + 1
	c.Version = frozenVersion
	c.Status = domain.StatusFrozen
	c.UpdatedAt = time.Now()
	manifest := domain.EvidenceManifest(c, bundle.Regions, bundle.Plan, bundle.Coupons, a, d)
	digest = domain.ManifestDigest(manifest)
	p := domain.WorkPermit{ID: fmt.Sprintf("permit-%d", time.Now().UnixNano()), CaseID: caseID, FrozenVersion: frozenVersion, ManifestDigest: digest, PermitCode: "WP-" + digest[:12], ApprovedBy: reviewer, IssuedAt: time.Now(), VerificationStatus: "valid"}
	if e = s.Repo.SavePermit(p); e != nil {
		return p, e
	}
	_, e = s.Repo.UpdateCase(c, expected, "")
	return p, e
}
func nonempty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
