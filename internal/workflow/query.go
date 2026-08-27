package workflow

import (
	"benzhiguji/internal/assessment"
	"benzhiguji/internal/domain"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

func (s *Service) Bundle(caseID string) (map[string]any, error) {
	b, e := s.Repo.Bundle(caseID)
	if e != nil {
		return nil, e
	}
	bound := map[string]bool{}
	for _, x := range b.Plan.RegionBindings {
		bound[x] = true
	}
	missing := []string{}
	for _, x := range b.Regions {
		if !bound[x.ID] && !bound[x.RegionCode] {
			missing = append(missing, x.RegionCode)
		}
	}
	trends, _ := s.CouponTrends(caseID)
	review, _ := s.Repo.LatestReview(caseID)
	return map[string]any{"case": b.Case, "regions": b.Regions, "plan": b.Plan, "plans": s.Repo.Plans(caseID), "coupons": b.Coupons, "couponTrends": trends, "assessment": b.Assessment, "events": b.Events, "permit": b.Permit, "review": review, "coverageMissing": missing}, nil
}
func (s *Service) ComparePlans(caseID string, fromNo, toNo int) (map[string]any, error) {
	ps := s.Repo.Plans(caseID)
	var a, b domain.TreatmentPlanRevision
	for _, p := range ps {
		if p.RevisionNo == fromNo {
			a = p
		}
		if p.RevisionNo == toNo {
			b = p
		}
	}
	if a.ID == "" || b.ID == "" {
		return nil, domain.ErrNotFound
	}
	diff := map[string]any{}
	if !reflect.DeepEqual(a.MaterialLots, b.MaterialLots) {
		diff["materials"] = compareList(a.MaterialLots, b.MaterialLots)
	}
	if !reflect.DeepEqual(a.ProcedureSteps, b.ProcedureSteps) {
		diff["procedures"] = compareList(a.ProcedureSteps, b.ProcedureSteps)
	}
	if !reflect.DeepEqual(a.RegionBindings, b.RegionBindings) {
		diff["regions"] = compareList(a.RegionBindings, b.RegionBindings)
	}
	if a.Constraints != b.Constraints {
		diff["constraints"] = map[string]any{"from": a.Constraints, "to": b.Constraints}
	}
	return map[string]any{"from": a, "to": b, "diff": diff}, nil
}
func compareList(a, b []string) map[string]any {
	am, bm := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		am[x] = true
	}
	for _, x := range b {
		bm[x] = true
	}
	added, removed, unchanged := []string{}, []string{}, []string{}
	for _, x := range b {
		if am[x] {
			unchanged = append(unchanged, x)
		} else {
			added = append(added, x)
		}
	}
	for _, x := range a {
		if !bm[x] {
			removed = append(removed, x)
		}
	}
	return map[string]any{"added": added, "removed": removed, "unchanged": unchanged, "from": a, "to": b}
}
func (s *Service) Timeline(caseID string) ([]domain.AuditEvent, error) { return s.Repo.Events(caseID) }
func (s *Service) CouponTrends(caseID string) (map[string]any, error) {
	cs, e := s.Repo.Coupons(caseID)
	if e != nil {
		return nil, e
	}
	chains := map[string][]domain.TrialCouponRevision{}
	for _, c := range cs {
		chains[c.CouponCode] = append(chains[c.CouponCode], c)
	}
	out := map[string]any{}
	for code, chain := range chains {
		if len(chain) == 0 {
			continue
		}
		sort.SliceStable(chain, func(i, j int) bool {
			if chain[i].RecordedAt.Equal(chain[j].RecordedAt) {
				return chain[i].RevisionNo < chain[j].RevisionNo
			}
			return chain[i].RecordedAt.Before(chain[j].RecordedAt)
		})
		first, last := chain[0], chain[len(chain)-1]
		out[code] = map[string]any{"revisions": chain, "latest": last.RevisionNo, "delta": map[string]any{"observationHours": last.ObservationHours - first.ObservationHours, "colorDelta": last.ColorDelta - first.ColorDelta, "ph": last.PHValue - first.PHValue, "peelStrength": last.PeelStrength - first.PeelStrength}}
	}
	return out, nil
}
func (s *Service) RiskSummary(caseID string) (map[string]any, error) {
	a, e := s.Repo.LatestAssessment(caseID)
	if e != nil {
		return nil, e
	}
	open := assessment.BlockingCodes(a.Findings)
	return map[string]any{"assessmentId": a.ID, "inputDigest": a.InputDigest, "openBlocking": open, "findingCount": len(a.Findings), "findingDigest": assessment.FindingsDigest(a.Findings)}, nil
}
func (s *Service) VerifyPermit(caseID, code string) (bool, error) {
	p, e := s.Repo.Permit(caseID)
	if e != nil {
		return false, e
	}
	c, e := s.Repo.GetCase(caseID)
	if e != nil {
		return false, e
	}
	b, _ := s.Repo.Bundle(caseID)
	rv, _ := s.Repo.LatestReview(caseID)
	digest := domain.ManifestDigest(domain.EvidenceManifest(b.Case, b.Regions, b.Plan, b.Coupons, b.Assessment, rv))
	return c.Status == domain.StatusFrozen && p.VerificationStatus == "valid" && strings.EqualFold(p.PermitCode, code) && digest == p.ManifestDigest, nil
}
func (s *Service) Reevaluate(caseID string, expected int) (domain.AssessmentSnapshot, error) {
	return s.Evaluate(caseID, expected)
}
func (s *Service) CloseWithEvidence(caseID string, expected int, evidence string) error {
	if len(strings.TrimSpace(evidence)) < 4 {
		return fmt.Errorf("%w: 整改证据至少四个字符", domain.ErrInvalid)
	}
	return s.Resolve(caseID, expected, evidence)
}
func (s *Service) PermitManifest(caseID string) (string, error) {
	if p, e := s.Repo.Permit(caseID); e == nil {
		return p.ManifestDigest, nil
	}
	b, e := s.Repo.Bundle(caseID)
	if e != nil {
		return "", e
	}
	rv, _ := s.Repo.LatestReview(caseID)
	return domain.ManifestDigest(domain.EvidenceManifest(b.Case, b.Regions, b.Plan, b.Coupons, b.Assessment, rv)), nil
}
func (s *Service) CandidateDigest(caseID string) (string, error) {
	s.candidateMu.Lock()
	defer s.candidateMu.Unlock()
	if digest, ok := s.candidateDigests[caseID]; ok {
		return digest, nil
	}
	b, e := s.Repo.Bundle(caseID)
	if e != nil {
		return "", e
	}
	digest := domain.ManifestDigest(struct {
		C domain.RestorationCase
		R []domain.DamageRegion
		P domain.TreatmentPlanRevision
		T []domain.TrialCouponRevision
		A domain.AssessmentSnapshot
	}{b.Case, b.Regions, b.Plan, b.Coupons, b.Assessment})
	s.candidateDigests[caseID] = digest
	return digest, nil
}
func (s *Service) PermitAge(caseID string) (time.Duration, error) {
	p, e := s.Repo.Permit(caseID)
	if e != nil {
		return 0, e
	}
	return time.Since(p.IssuedAt), nil
}
