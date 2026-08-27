package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound   = errors.New("未找到资源")
	ErrConflict   = errors.New("版本冲突")
	ErrInvalid    = errors.New("输入不合法")
	ErrFrozen     = errors.New("案卷已冻结")
	ErrTransition = errors.New("状态迁移不合法")
)

type CaseStatus string

const (
	StatusDraft       CaseStatus = "draft"
	StatusPlanning    CaseStatus = "planning"
	StatusEvaluated   CaseStatus = "evaluated"
	StatusRemediation CaseStatus = "remediation"
	StatusReview      CaseStatus = "review"
	StatusApproved    CaseStatus = "approved"
	StatusFrozen      CaseStatus = "frozen"
)

type RestorationCase struct {
	ID, CollectionCode, Title, MaterialProfile, OwnerName, TargetDate string
	Status                                                            CaseStatus
	Version                                                           int
	CreatedAt, UpdatedAt                                              time.Time
}
type DamageRegion struct {
	ID, CaseID, RegionCode, Location, DamageType, EvidenceNote string
	Severity                                                   string
	WidthMM, HeightMM                                          float64
	Revision                                                   int
}
type TreatmentPlanRevision struct {
	ID, CaseID, AuthorName                       string
	RevisionNo                                   int
	MaterialLots, ProcedureSteps, RegionBindings []string
	Constraints                                  string
	CreatedAt                                    time.Time
}
type TrialCouponRevision struct {
	ID, CaseID, PlanRevisionID, CouponCode, Substrate, Formula, Environment string
	ObservationHours                                                        int
	ColorDelta, PHValue, PeelStrength                                       float64
	ReversibilityGrade                                                      string
	RevisionNo                                                              int
	RecordedAt                                                              time.Time
}
type RiskFinding struct {
	ID, AssessmentID, RuleCode, Severity, Basis, SuggestedAction, Status, ResolutionEvidence string
	RegionIDs                                                                                []string
	ResolvedAt                                                                               *time.Time
}
type AssessmentSnapshot struct {
	ID, CaseID, PlanRevisionID, InputDigest string
	Findings                                []RiskFinding
	InputSummary                            map[string]any `json:"inputSummary,omitempty"`
	CreatedAt                               time.Time
}
type ReviewDecision struct {
	ID, CaseID, CandidateDigest, Reviewer, Decision, Reason string
	CreatedAt                                               time.Time
}
type WorkPermit struct {
	ID, CaseID                                                 string
	FrozenVersion                                              int
	ManifestDigest, PermitCode, ApprovedBy, VerificationStatus string
	IssuedAt                                                   time.Time
}
type WarningLevel string

const (
	WarningNone    WarningLevel = "none"
	WarningDue     WarningLevel = "due"
	WarningOverdue WarningLevel = "overdue"
)

type AuditEvent struct {
	ID, CaseID, Type, Detail string
	At                       time.Time
}

func ValidateCase(c RestorationCase) error {
	if strings.TrimSpace(c.CollectionCode) == "" || strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.OwnerName) == "" || c.TargetDate == "" {
		return fmt.Errorf("%w: 藏品标识、标题、责任人和目标日期必填", ErrInvalid)
	}
	if _, e := time.Parse("2006-01-02", c.TargetDate); e != nil {
		return fmt.Errorf("%w: 目标日期必须使用 YYYY-MM-DD", ErrInvalid)
	}
	return nil
}
func ValidateRegion(r DamageRegion) error {
	if strings.TrimSpace(r.RegionCode) == "" || strings.TrimSpace(r.Location) == "" || strings.TrimSpace(r.DamageType) == "" || strings.TrimSpace(r.Severity) == "" || !ValidSeverity(r.Severity) || r.WidthMM <= 0 || r.HeightMM <= 0 {
		return fmt.Errorf("%w: 病害区域测量和说明必须完整", ErrInvalid)
	}
	return nil
}
func ValidatePlan(p TreatmentPlanRevision, regions []DamageRegion) error {
	if len(p.MaterialLots) == 0 || len(p.ProcedureSteps) == 0 || len(p.RegionBindings) == 0 {
		return fmt.Errorf("%w: 方案必须包含材料、工序和病害绑定", ErrInvalid)
	}
	for _, v := range append(append(append([]string{}, p.MaterialLots...), p.ProcedureSteps...), p.RegionBindings...) {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%w: 方案条目不能为空", ErrInvalid)
		}
	}
	known := map[string]bool{}
	for _, r := range regions {
		if r.ID != "" {
			known[r.ID] = true
		}
		known[r.RegionCode] = true
	}
	bindings := map[string]bool{}
	for _, id := range p.RegionBindings {
		if strings.TrimSpace(id) == "" || !known[id] {
			return fmt.Errorf("%w: 方案绑定了未知病害区域", ErrInvalid)
		}
		if bindings[id] {
			return fmt.Errorf("%w: 方案病害绑定重复", ErrInvalid)
		}
		bindings[id] = true
	}
	for _, r := range regions {
		if !knownBinding(p.RegionBindings, r.ID, r.RegionCode) {
			return fmt.Errorf("%w: 区域 %s 未被方案覆盖", ErrInvalid, r.RegionCode)
		}
	}
	constraints := strings.ToLower(p.Constraints)
	if strings.Contains(constraints, "冲突") || strings.Contains(constraints, "conflict") || strings.Contains(constraints, "互斥") {
		return fmt.Errorf("%w: 适用边界包含冲突条目", ErrInvalid)
	}
	return nil
}
func knownBinding(bindings []string, values ...string) bool {
	for _, b := range bindings {
		for _, v := range values {
			if b == v {
				return true
			}
		}
	}
	return false
}
func ValidateCoupon(c TrialCouponRevision) error {
	if c.CouponCode == "" || c.Substrate == "" || c.Formula == "" || c.Environment == "" || c.ObservationHours <= 0 || c.PHValue <= 0 || c.PeelStrength <= 0 || c.ColorDelta < 0 || c.ReversibilityGrade == "" {
		return fmt.Errorf("%w: 试片字段或测量不完整", ErrInvalid)
	}
	return nil
}
func CanTransition(from, to CaseStatus) bool {
	switch from {
	case StatusDraft:
		return to == StatusPlanning
	case StatusPlanning:
		return to == StatusEvaluated
	case StatusEvaluated:
		return to == StatusRemediation || to == StatusReview
	case StatusRemediation:
		return to == StatusEvaluated || to == StatusReview
	case StatusReview:
		return to == StatusApproved || to == StatusRemediation
	case StatusApproved:
		return to == StatusFrozen
	}
	return false
}
func ManifestDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func DateWarning(target string, now time.Time, window int) (WarningLevel, int) {
	parsed, err := time.Parse("2006-01-02", target)
	if err != nil {
		return WarningNone, 0
	}
	n := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	t := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, now.Location())
	d := int(t.Sub(n).Hours() / 24)
	if d <= 0 {
		return WarningOverdue, d
	}
	if d <= window {
		return WarningDue, d
	}
	return WarningNone, d
}
