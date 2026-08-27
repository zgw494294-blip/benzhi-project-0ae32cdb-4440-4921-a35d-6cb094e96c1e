package domain

import (
	"fmt"
	"strings"
	"time"
)

type EventType string

const (
	EventCaseCreated EventType = "case.created"
	EventCaseUpdated EventType = "case.updated"
	EventRegionAdded EventType = "region.added"
	EventPlanAdded   EventType = "plan.added"
	EventCouponAdded EventType = "coupon.added"
	EventAssessment  EventType = "assessment.created"
	EventRiskClosed  EventType = "risks.closed"
	EventReview      EventType = "review"
	EventPermit      EventType = "permit.issued"
)

func NewEvent(caseID string, typ EventType, detail string) AuditEvent {
	return AuditEvent{ID: fmt.Sprintf("evt-%d", time.Now().UnixNano()), CaseID: caseID, Type: string(typ), Detail: strings.TrimSpace(detail), At: time.Now()}
}
func (e AuditEvent) Valid() bool {
	return e.ID != "" && e.CaseID != "" && e.Type != "" && !e.At.IsZero()
}
func (e AuditEvent) Label() string {
	switch EventType(e.Type) {
	case EventCaseCreated:
		return "创建案卷"
	case EventCaseUpdated:
		return "更新状态"
	case EventRegionAdded:
		return "登记病害"
	case EventPlanAdded:
		return "提交方案"
	case EventCouponAdded:
		return "记录试片"
	case EventAssessment:
		return "完成评估"
	case EventRiskClosed:
		return "关闭风险"
	case EventReview:
		return "复核决定"
	case EventPermit:
		return "签发许可"
	}
	return e.Type
}
func SortEvents(es []AuditEvent) {
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j].At.Before(es[j-1].At); j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
}
func EventDigest(es []AuditEvent) string {
	v := []string{}
	for _, e := range es {
		v = append(v, e.ID+"|"+e.Type+"|"+e.Detail+"|"+e.At.Format(time.RFC3339Nano))
	}
	return ManifestDigest(v)
}
