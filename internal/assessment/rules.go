package assessment

import (
	"benzhiguji/internal/domain"
	"fmt"
	"sort"
	"strings"
)

type Rule struct {
	Code        string
	Severity    string
	Description string
	Check       func(domain.TrialCouponRevision) bool
	Action      string
}

func (e Evaluator) Rules() []Rule {
	return []Rule{{"OBSERVATION_SHORT", "blocking", "观察周期不足", func(x domain.TrialCouponRevision) bool { return x.ObservationHours >= e.MinObservation }, "补充达到规定周期的观察记录"}, {"COLOR_DELTA_HIGH", "blocking", "色差超限", func(x domain.TrialCouponRevision) bool { return x.ColorDelta <= e.MaxColorDelta }, "调整配方并重新制作试片"}, {"PH_OUT_OF_RANGE", "blocking", "酸碱度不在安全范围", func(x domain.TrialCouponRevision) bool { return x.PHValue >= e.MinPH }, "更换或缓冲材料后复验"}, {"PEEL_WEAK", "warning", "剥离强度偏低", func(x domain.TrialCouponRevision) bool { return x.PeelStrength >= e.MinPeel }, "增加加固验证并由复核员评估"}, {"REVERSIBILITY_POOR", "blocking", "可逆性不足", func(x domain.TrialCouponRevision) bool {
		return !strings.EqualFold(x.ReversibilityGrade, "差") && !strings.EqualFold(x.ReversibilityGrade, "poor") && !strings.EqualFold(x.ReversibilityGrade, "bad")
	}, "选择可逆材料并补充试片"}}
}
func (e Evaluator) EvaluateCoupon(x domain.TrialCouponRevision, regions []string) []domain.RiskFinding {
	o := []domain.RiskFinding{}
	for _, r := range e.Rules() {
		if !r.Check(x) {
			o = append(o, domain.RiskFinding{RuleCode: r.Code, Severity: r.Severity, Basis: fmt.Sprintf("试片 %s：%s", x.CouponCode, r.Description), SuggestedAction: r.Action, RegionIDs: regions, Status: "open"})
		}
	}
	return o
}
func SortFindings(fs []domain.RiskFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		ri, rj := domain.SeverityRank(fs[i].Severity), domain.SeverityRank(fs[j].Severity)
		if ri != rj {
			return ri > rj
		}
		if fs[i].RuleCode != fs[j].RuleCode {
			return fs[i].RuleCode < fs[j].RuleCode
		}
		return fs[i].Basis < fs[j].Basis
	})
}
func BlockingCodes(fs []domain.RiskFinding) []string {
	o := []string{}
	for _, f := range fs {
		if f.Severity == "blocking" && f.Status != "closed" {
			o = append(o, f.RuleCode)
		}
	}
	sort.Strings(o)
	return o
}
func FindingsDigest(fs []domain.RiskFinding) string {
	type item struct{ Code, Severity, Basis, Action, Status string }
	SortFindings(fs)
	v := []item{}
	for _, f := range fs {
		v = append(v, item{f.RuleCode, f.Severity, f.Basis, f.SuggestedAction, f.Status})
	}
	return domain.ManifestDigest(v)
}
func CanClose(fs []domain.RiskFinding, evidence string) bool {
	if strings.TrimSpace(evidence) == "" {
		return false
	}
	for _, f := range fs {
		if f.Severity == "blocking" && f.Status != "closed" {
			return false
		}
	}
	return true
}
