package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func StatusLabel(s CaseStatus) string {
	switch s {
	case StatusDraft:
		return "草稿"
	case StatusPlanning:
		return "方案编制"
	case StatusEvaluated:
		return "已评估"
	case StatusRemediation:
		return "整改中"
	case StatusReview:
		return "待复核"
	case StatusApproved:
		return "已批准"
	case StatusFrozen:
		return "已冻结"
	}
	return "未知"
}
func Statuses() []CaseStatus {
	return []CaseStatus{StatusDraft, StatusPlanning, StatusEvaluated, StatusRemediation, StatusReview, StatusApproved, StatusFrozen}
}
func SeverityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "blocking", "high", "阻断", "高":
		return 3
	case "warning", "medium", "警告", "中":
		return 2
	case "info", "low", "提示", "低":
		return 1
	}
	return 0
}
func ValidSeverity(s string) bool { return SeverityRank(s) > 0 }
func (c RestorationCase) Summary() map[string]any {
	return map[string]any{"id": c.ID, "title": c.Title, "status": c.Status, "statusLabel": StatusLabel(c.Status), "version": c.Version, "targetDate": c.TargetDate}
}
func (r DamageRegion) AreaMM2() float64 { return r.WidthMM * r.HeightMM }
func (r DamageRegion) Summary() map[string]any {
	return map[string]any{"id": r.ID, "code": r.RegionCode, "location": r.Location, "type": r.DamageType, "severity": r.Severity, "areaMM2": r.AreaMM2()}
}
func (p TreatmentPlanRevision) Summary() map[string]any {
	return map[string]any{"id": p.ID, "revision": p.RevisionNo, "materials": p.MaterialLots, "procedures": p.ProcedureSteps, "regions": p.RegionBindings, "constraints": p.Constraints}
}
func (x TrialCouponRevision) Summary() map[string]any {
	return map[string]any{"id": x.ID, "code": x.CouponCode, "observationHours": x.ObservationHours, "colorDelta": x.ColorDelta, "ph": x.PHValue, "peelStrength": x.PeelStrength, "reversibility": x.ReversibilityGrade}
}
func NormalizeList(v []string) []string {
	seen := map[string]bool{}
	o := []string{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x != "" && !seen[x] {
			seen[x] = true
			o = append(o, x)
		}
	}
	sort.Strings(o)
	return o
}
func EvidenceManifest(c RestorationCase, rs []DamageRegion, p TreatmentPlanRevision, cs []TrialCouponRevision, a AssessmentSnapshot, d ReviewDecision) map[string]any {
	return map[string]any{"case": c.Summary(), "regions": rs, "plan": p.Summary(), "coupons": cs, "assessment": a, "review": d}
}
func CanonicalJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func ValidateAggregate(c RestorationCase, rs []DamageRegion, p *TreatmentPlanRevision, cs []TrialCouponRevision) error {
	if e := ValidateCase(c); e != nil {
		return e
	}
	if len(rs) > 0 {
		codes := map[string]bool{}
		for _, r := range rs {
			if e := ValidateRegion(r); e != nil {
				return e
			}
			if codes[r.RegionCode] {
				return fmt.Errorf("%w: 区域编号重复", ErrInvalid)
			}
			codes[r.RegionCode] = true
		}
	}
	if p != nil {
		if e := ValidatePlan(*p, rs); e != nil {
			return e
		}
	}
	for _, x := range cs {
		if e := ValidateCoupon(x); e != nil {
			return e
		}
	}
	return nil
}
