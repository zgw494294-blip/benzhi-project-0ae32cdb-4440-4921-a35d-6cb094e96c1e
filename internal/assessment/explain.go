package assessment

import (
	"benzhiguji/internal/domain"
	"fmt"
	"strings"
)

type Result struct {
	RuleCode string `json:"ruleCode"`
	Passed   bool   `json:"passed"`
	Severity string `json:"severity"`
	Basis    string `json:"basis"`
	Action   string `json:"action"`
}

func (e Evaluator) Explain(x domain.TrialCouponRevision) []Result {
	o := []Result{}
	for _, r := range e.Rules() {
		ok := r.Check(x)
		basis := fmt.Sprintf("试片 %s：%s", x.CouponCode, r.Description)
		switch r.Code {
		case "COLOR_DELTA_HIGH":
			basis = fmt.Sprintf("色差 %.2f，阈值 %.2f", x.ColorDelta, e.MaxColorDelta)
		case "PH_OUT_OF_RANGE":
			basis = fmt.Sprintf("pH %.2f，下限 %.2f", x.PHValue, e.MinPH)
		case "PEEL_WEAK":
			basis = fmt.Sprintf("剥离强度 %.2f，下限 %.2f", x.PeelStrength, e.MinPeel)
		case "OBSERVATION_SHORT":
			basis = fmt.Sprintf("观察 %d 小时，要求 %d 小时", x.ObservationHours, e.MinObservation)
		}
		o = append(o, Result{r.Code, ok, r.Severity, basis, r.Action})
	}
	return o
}
func ExplainSnapshot(s domain.AssessmentSnapshot) []string {
	o := []string{}
	for _, f := range s.Findings {
		state := "未关闭"
		if f.Status == "closed" {
			state = "已关闭"
		}
		o = append(o, fmt.Sprintf("[%s] %s：%s（%s）", f.Severity, f.RuleCode, f.Basis, state))
	}
	return o
}
func NormalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "高", "阻断", "blocking", "high":
		return "blocking"
	case "中", "警告", "warning", "medium":
		return "warning"
	case "低", "提示", "info", "low":
		return "info"
	default:
		return "info"
	}
}
func OpenFindings(s domain.AssessmentSnapshot) []domain.RiskFinding {
	o := []domain.RiskFinding{}
	for _, f := range s.Findings {
		if f.Status != "closed" {
			o = append(o, f)
		}
	}
	SortFindings(o)
	return o
}
func SeverityCounts(fs []domain.RiskFinding) map[string]int {
	o := map[string]int{"blocking": 0, "warning": 0, "info": 0}
	for _, f := range fs {
		o[NormalizeSeverity(f.Severity)]++
	}
	return o
}
func MergeEvidence(old, new string) string {
	old = strings.TrimSpace(old)
	new = strings.TrimSpace(new)
	if old == "" {
		return new
	}
	if new == "" {
		return old
	}
	return old + "；" + new
}
