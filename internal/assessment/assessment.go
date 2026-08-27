package assessment

import (
	"benzhiguji/internal/domain"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Evaluator struct {
	MaxColorDelta, MinPH, MinPeel float64
	MinObservation                int
}

func DefaultEvaluator() Evaluator {
	return Evaluator{MaxColorDelta: 5, MinPH: 5, MinPeel: 1.5, MinObservation: 24}
}
func (e Evaluator) Evaluate(c domain.RestorationCase, p domain.TreatmentPlanRevision, rs []domain.DamageRegion, cs []domain.TrialCouponRevision) domain.AssessmentSnapshot {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].RecordedAt.Equal(cs[j].RecordedAt) {
			if cs[i].CouponCode == cs[j].CouponCode {
				return cs[i].RevisionNo > cs[j].RevisionNo
			}
			return cs[i].CouponCode < cs[j].CouponCode
		}
		return cs[i].RecordedAt.After(cs[j].RecordedAt)
	})
	findings := []domain.RiskFinding{}
	ids := []string{}
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	latest := map[string]domain.TrialCouponRevision{}
	for _, x := range cs {
		if old, ok := latest[x.CouponCode]; !ok || x.RevisionNo > old.RevisionNo || (x.RevisionNo == old.RevisionNo && x.RecordedAt.After(old.RecordedAt)) {
			latest[x.CouponCode] = x
		}
	}
	if len(cs) == 0 {
		findings = append(findings, e.finding("TRIAL_MISSING", "blocking", "没有可用试片证据", "至少完成一项覆盖方案的试片并记录完整测量", ids))
	}
	for code, x := range latest {
		if x.ObservationHours < e.MinObservation {
			findings = append(findings, e.finding("OBSERVATION_SHORT", "blocking", fmt.Sprintf("试片 %s 观察周期为 %d 小时", code, x.ObservationHours), "补充达到规定周期的观察记录", ids))
		}
		if x.ColorDelta > e.MaxColorDelta {
			findings = append(findings, e.finding("COLOR_DELTA_HIGH", "blocking", fmt.Sprintf("试片 %s 色差 %.2f 超过 %.2f", code, x.ColorDelta, e.MaxColorDelta), "调整配方并重新制作试片", ids))
		}
		if x.PHValue < e.MinPH {
			findings = append(findings, e.finding("PH_OUT_OF_RANGE", "blocking", fmt.Sprintf("试片 %s pH %.2f 低于 %.2f", code, x.PHValue, e.MinPH), "更换或缓冲材料后复验", ids))
		}
		if x.PeelStrength < e.MinPeel {
			findings = append(findings, e.finding("PEEL_WEAK", "warning", fmt.Sprintf("试片 %s 剥离强度 %.2f 偏低", code, x.PeelStrength), "增加加固验证并由复核员评估", ids))
		}
		if strings.EqualFold(x.ReversibilityGrade, "差") || strings.EqualFold(x.ReversibilityGrade, "poor") || strings.EqualFold(x.ReversibilityGrade, "bad") {
			findings = append(findings, e.finding("REVERSIBILITY_POOR", "blocking", fmt.Sprintf("试片 %s 可逆性为差", code), "选择可逆材料并补充试片", ids))
		}
	}
	for i := range findings {
		findings[i].AssessmentID = "pending"
		findings[i].ID = fmt.Sprintf("risk-%d", i+1)
	}
	SortFindings(findings)
	latestList := make([]domain.TrialCouponRevision, 0, len(latest))
	for _, x := range latest {
		latestList = append(latestList, x)
	}
	sort.Slice(latestList, func(i, j int) bool { return latestList[i].CouponCode < latestList[j].CouponCode })
	input := domain.ManifestDigest(struct {
		C string
		P domain.TreatmentPlanRevision
		R []domain.DamageRegion
		T []domain.TrialCouponRevision
	}{c.ID, p, rs, latestList})
	regions := make([]string, 0, len(rs))
	for _, r := range rs {
		regions = append(regions, r.ID)
	}
	summary := map[string]any{"planRevisionId": p.ID, "regionIds": regions, "coupons": latestList}
	return domain.AssessmentSnapshot{ID: fmt.Sprintf("assessment-%d", time.Now().UnixNano()), CaseID: c.ID, PlanRevisionID: p.ID, InputDigest: input, InputSummary: summary, Findings: findings, CreatedAt: time.Now()}
}
func (e Evaluator) finding(code, sev, basis, action string, regions []string) domain.RiskFinding {
	return domain.RiskFinding{RuleCode: code, Severity: sev, Basis: basis, SuggestedAction: action, RegionIDs: regions, Status: "open"}
}
func HasBlockingOpen(s domain.AssessmentSnapshot) bool {
	for _, f := range s.Findings {
		if f.Severity == "blocking" && f.Status != "closed" {
			return true
		}
	}
	return false
}
