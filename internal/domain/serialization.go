package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }
func Require(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return FieldError{field, "不能为空"}
	}
	return nil
}
func ParsePositive(value, field string) (float64, error) {
	v, e := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if e != nil || v <= 0 {
		return 0, FieldError{field, "必须是正数"}
	}
	return v, nil
}
func ParseDate(value string) (time.Time, error) {
	t, e := time.Parse("2006-01-02", value)
	if e != nil {
		return time.Time{}, FieldError{"targetDate", "必须使用 YYYY-MM-DD"}
	}
	return t, nil
}
func CheckTargetDate(value string) error {
	t, e := ParseDate(value)
	if e != nil {
		return e
	}
	if t.Before(time.Now().Truncate(24 * time.Hour)) {
		return errors.New("targetDate: 目标日期不能早于今天")
	}
	return nil
}
func EnsureUniqueRegions(rs []DamageRegion) error {
	seen := map[string]bool{}
	for _, r := range rs {
		if seen[r.RegionCode] {
			return fmt.Errorf("%w: 区域编号 %s 重复", ErrInvalid, r.RegionCode)
		}
		seen[r.RegionCode] = true
	}
	return nil
}
func EnsurePlanCoverage(p TreatmentPlanRevision, rs []DamageRegion) error {
	bound := map[string]bool{}
	for _, x := range p.RegionBindings {
		bound[x] = true
	}
	for _, r := range rs {
		if !bound[r.ID] && !bound[r.RegionCode] {
			return fmt.Errorf("%w: 区域 %s 未被方案覆盖", ErrInvalid, r.RegionCode)
		}
	}
	return nil
}
func EnsureCouponLink(x TrialCouponRevision, p TreatmentPlanRevision) error {
	if x.PlanRevisionID != "" && x.PlanRevisionID != p.ID {
		return fmt.Errorf("%w: 试片不属于当前方案", ErrInvalid)
	}
	return nil
}
func FreezeGuard(c RestorationCase) error {
	if c.Status == StatusFrozen {
		return ErrFrozen
	}
	return nil
}
func DecisionValid(decision, reason string) error {
	if decision != "approve" && decision != "reject" {
		return FieldError{"decision", "只能是 approve 或 reject"}
	}
	if decision == "reject" && strings.TrimSpace(reason) == "" {
		return FieldError{"reason", "退回必须说明原因"}
	}
	return nil
}
func IsTerminal(s CaseStatus) bool { return s == StatusFrozen }
func IsEditable(s CaseStatus) bool { return s != StatusFrozen }
