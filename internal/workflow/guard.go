package workflow

import (
	"benzhiguji/internal/assessment"
	"benzhiguji/internal/domain"
	"fmt"
	"strings"
	"time"
)

type Permission struct {
	Role    string
	Actions []string
}

func Permissions(role string) Permission {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "修复师", "restorer":
		return Permission{role, []string{"create", "region", "plan", "resolve", "review"}}
	case "试验员", "tester":
		return Permission{role, []string{"coupon"}}
	case "复核员", "reviewer":
		return Permission{role, []string{"decide", "verify"}}
	}
	return Permission{role, nil}
}
func Can(role, action string) bool {
	for _, x := range Permissions(role).Actions {
		if x == action {
			return true
		}
	}
	return false
}
func CheckExpected(c domain.RestorationCase, expected int) error {
	if expected < 1 {
		return fmt.Errorf("%w: expectedVersion 必须为正数", domain.ErrInvalid)
	}
	if c.Version != expected {
		return domain.ErrConflict
	}
	return nil
}
func CheckReviewable(c domain.RestorationCase, a domain.AssessmentSnapshot) error {
	if c.Status != domain.StatusEvaluated && c.Status != domain.StatusRemediation {
		return domain.ErrTransition
	}
	if assessment.HasBlockingOpen(a) {
		return fmt.Errorf("%w: 仍有未关闭阻断风险", domain.ErrInvalid)
	}
	return nil
}
func CheckApproval(c domain.RestorationCase, a domain.AssessmentSnapshot, digest string) error {
	if c.Status != domain.StatusReview {
		return domain.ErrTransition
	}
	if assessment.HasBlockingOpen(a) {
		return fmt.Errorf("%w: 候选证据存在阻断项", domain.ErrInvalid)
	}
	if digest == "" {
		return fmt.Errorf("%w: 候选摘要为空", domain.ErrInvalid)
	}
	return nil
}
func ReviewExpiry(created time.Time, ttl time.Duration) bool {
	if created.IsZero() {
		return true
	}
	return time.Since(created) > ttl
}
func DescribeTransition(from, to domain.CaseStatus) string {
	return domain.StatusLabel(from) + " -> " + domain.StatusLabel(to)
}
func RequireRole(role, action string) error {
	if !Can(role, action) {
		return fmt.Errorf("%w: 角色 %s 无权执行 %s", domain.ErrInvalid, role, action)
	}
	return nil
}
