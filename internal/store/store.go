package store

import (
	"benzhiguji/internal/domain"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

type snapshot struct {
	Cases       map[string]domain.RestorationCase       `json:"cases"`
	Regions     map[string]domain.DamageRegion          `json:"regions"`
	Plans       map[string]domain.TreatmentPlanRevision `json:"plans"`
	Coupons     map[string]domain.TrialCouponRevision   `json:"coupons"`
	Assessments map[string]domain.AssessmentSnapshot    `json:"assessments"`
	Reviews     map[string]domain.ReviewDecision        `json:"reviews"`
	Permits     map[string]domain.WorkPermit            `json:"permits"`
	Events      []domain.AuditEvent                     `json:"events"`
	Idempotency map[string]string                       `json:"idempotency"`
}
type Repository struct {
	mu   sync.Mutex
	path string
	s    snapshot
}

func Open(path string) (*Repository, error) {
	if path == "" {
		path = "guji.db"
	}
	r := &Repository{path: path}
	r.init()
	b, e := os.ReadFile(path)
	if e == nil && len(b) > 0 {
		if e = json.Unmarshal(b, &r.s); e != nil {
			return nil, e
		}
		r.initMaps()
	}
	return r, nil
}
func (r *Repository) init() {
	r.s.Cases = map[string]domain.RestorationCase{}
	r.s.Regions = map[string]domain.DamageRegion{}
	r.s.Plans = map[string]domain.TreatmentPlanRevision{}
	r.s.Coupons = map[string]domain.TrialCouponRevision{}
	r.s.Assessments = map[string]domain.AssessmentSnapshot{}
	r.s.Reviews = map[string]domain.ReviewDecision{}
	r.s.Permits = map[string]domain.WorkPermit{}
	r.s.Events = []domain.AuditEvent{}
	r.s.Idempotency = map[string]string{}
}
func (r *Repository) persist() {
	b, _ := json.MarshalIndent(r.s, "", "  ")
	_ = os.WriteFile(r.path, b, 0600)
}
func (r *Repository) Close() error { r.mu.Lock(); defer r.mu.Unlock(); r.persist(); return nil }
func (r *Repository) CreateCase(c domain.RestorationCase, key string) (domain.RestorationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.s.Idempotency[key]; key != "" && ok {
		_ = json.Unmarshal([]byte(v), &c)
		return c, nil
	}
	c.Version = 1
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	r.s.Cases[c.ID] = c
	r.addEvent(c.ID, "case.created", "案卷已创建")
	r.saveKey(key, c)
	r.persist()
	return c, nil
}
func (r *Repository) GetCase(id string) (domain.RestorationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.s.Cases[id]
	if !ok {
		return domain.RestorationCase{}, domain.ErrNotFound
	}
	return c, nil
}
func (r *Repository) UpdateCase(c domain.RestorationCase, expected int, key string) (domain.RestorationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.s.Idempotency[key]; key != "" && ok {
		_ = json.Unmarshal([]byte(v), &c)
		return c, nil
	}
	old, ok := r.s.Cases[c.ID]
	if !ok {
		return c, domain.ErrNotFound
	}
	if old.Version != expected {
		return c, domain.ErrConflict
	}
	if old.Status == domain.StatusFrozen {
		return c, domain.ErrFrozen
	}
	c.Version = old.Version + 1
	c.UpdatedAt = time.Now()
	r.s.Cases[c.ID] = c
	r.addEvent(c.ID, "case.updated", string(c.Status))
	r.saveKey(key, c)
	r.persist()
	return c, nil
}
func (r *Repository) AddRegion(x domain.DamageRegion) (domain.DamageRegion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.Regions[x.ID] = x
	r.addEvent(x.CaseID, "region.added", x.RegionCode)
	r.persist()
	return x, nil
}
func (r *Repository) AddRegions(xs []domain.DamageRegion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range xs {
		if x.ID == "" {
			x.ID = fmt.Sprintf("region-%d-%d", time.Now().UnixNano(), i)
		}
		r.s.Regions[x.ID] = x
		r.addEvent(x.CaseID, "region.added", x.RegionCode)
	}
	r.persist()
	return nil
}
func (r *Repository) AddRegionsAndBump(caseID string, xs []domain.DamageRegion, expected int, key string) (domain.RestorationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.s.Idempotency[key]; key != "" && ok {
		var c domain.RestorationCase
		if json.Unmarshal([]byte(v), &c) == nil {
			return c, nil
		}
	}
	c, ok := r.s.Cases[caseID]
	if !ok {
		return c, domain.ErrNotFound
	}
	if c.Version != expected {
		return c, domain.ErrConflict
	}
	if c.Status == domain.StatusFrozen {
		return c, domain.ErrFrozen
	}
	for i, x := range xs {
		if x.ID == "" {
			x.ID = fmt.Sprintf("region-%d-%d", time.Now().UnixNano(), i)
		}
		r.s.Regions[x.ID] = x
		r.addEvent(caseID, "region.added", x.RegionCode)
	}
	c.Version++
	c.UpdatedAt = time.Now()
	r.s.Cases[caseID] = c
	r.addEvent(caseID, "case.updated", string(c.Status))
	r.saveKey(key, c)
	r.persist()
	return c, nil
}
func (r *Repository) Regions(id string) ([]domain.DamageRegion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := []domain.DamageRegion{}
	for _, x := range r.s.Regions {
		if x.CaseID == id {
			o = append(o, x)
		}
	}
	sort.Slice(o, func(i, j int) bool {
		if o[i].RegionCode == o[j].RegionCode {
			return o[i].ID < o[j].ID
		}
		return o[i].RegionCode < o[j].RegionCode
	})
	return o, nil
}
func (r *Repository) AddPlan(x domain.TreatmentPlanRevision) (domain.TreatmentPlanRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x = clonePlan(x)
	r.s.Plans[x.ID] = x
	r.addEvent(x.CaseID, "plan.added", x.ID)
	r.persist()
	return x, nil
}
func (r *Repository) AddPlanAndBump(x domain.TreatmentPlanRevision, expected int) (domain.TreatmentPlanRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.s.Cases[x.CaseID]
	if !ok {
		return x, domain.ErrNotFound
	}
	if c.Version != expected {
		return x, domain.ErrConflict
	}
	if c.Status == domain.StatusFrozen {
		return x, domain.ErrFrozen
	}
	if x.ID == "" {
		x.ID = "plan-" + fmt.Sprint(time.Now().UnixNano())
	}
	x.MaterialLots = append([]string(nil), x.MaterialLots...)
	x.ProcedureSteps = append([]string(nil), x.ProcedureSteps...)
	x.RegionBindings = append([]string(nil), x.RegionBindings...)
	if _, exists := r.s.Plans[x.ID]; exists {
		return x, fmt.Errorf("%w: 方案标识重复", domain.ErrInvalid)
	}
	max := 0
	for _, v := range r.s.Plans {
		if v.CaseID == x.CaseID && v.RevisionNo > max {
			max = v.RevisionNo
		}
	}
	x.RevisionNo = max + 1
	r.s.Plans[x.ID] = x
	r.addEvent(x.CaseID, "plan.added", x.ID)
	if c.Status == domain.StatusDraft {
		c.Status = domain.StatusPlanning
	}
	c.Version++
	c.UpdatedAt = time.Now()
	r.s.Cases[x.CaseID] = c
	r.addEvent(x.CaseID, "case.updated", string(c.Status))
	r.persist()
	return x, nil
}
func (r *Repository) Plans(id string) []domain.TreatmentPlanRevision {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.TreatmentPlanRevision{}
	for _, x := range r.s.Plans {
		if x.CaseID == id {
			out = append(out, clonePlan(x))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RevisionNo < out[j].RevisionNo })
	return out
}
func (r *Repository) LatestPlan(id string) (domain.TreatmentPlanRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out domain.TreatmentPlanRevision
	found := false
	for _, x := range r.s.Plans {
		if x.CaseID == id && (!found || x.RevisionNo > out.RevisionNo || (x.RevisionNo == out.RevisionNo && x.ID > out.ID)) {
			out = x
			found = true
		}
	}
	if !found {
		return out, domain.ErrNotFound
	}
	return clonePlan(out), nil
}
func clonePlan(p domain.TreatmentPlanRevision) domain.TreatmentPlanRevision {
	p.MaterialLots = append([]string(nil), p.MaterialLots...)
	p.ProcedureSteps = append([]string(nil), p.ProcedureSteps...)
	p.RegionBindings = append([]string(nil), p.RegionBindings...)
	return p
}
func (r *Repository) AddCoupon(x domain.TrialCouponRevision) (domain.TrialCouponRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.Coupons[x.ID] = x
	r.addEvent(x.CaseID, "coupon.added", x.CouponCode)
	r.persist()
	return x, nil
}
func (r *Repository) AddCouponAndBump(x domain.TrialCouponRevision, expected int) (domain.TrialCouponRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.s.Cases[x.CaseID]
	if !ok {
		return x, domain.ErrNotFound
	}
	if c.Version != expected {
		return x, domain.ErrConflict
	}
	if c.Status == domain.StatusFrozen {
		return x, domain.ErrFrozen
	}
	if x.ID == "" {
		x.ID = "coupon-" + fmt.Sprint(time.Now().UnixNano())
	}
	if _, exists := r.s.Coupons[x.ID]; exists {
		return x, fmt.Errorf("%w: 试片修订标识重复", domain.ErrInvalid)
	}
	max := 0
	for _, v := range r.s.Coupons {
		if v.CaseID == x.CaseID && v.CouponCode == x.CouponCode && v.RevisionNo > max {
			max = v.RevisionNo
		}
	}
	x.RevisionNo = max + 1
	r.s.Coupons[x.ID] = x
	r.addEvent(x.CaseID, "coupon.added", x.CouponCode)
	c.Version++
	c.UpdatedAt = time.Now()
	r.s.Cases[x.CaseID] = c
	r.addEvent(x.CaseID, "case.updated", string(c.Status))
	r.persist()
	return x, nil
}
func (r *Repository) LatestCouponByCode(caseID, code string) (domain.TrialCouponRevision, error) {
	cs, _ := r.Coupons(caseID)
	var out domain.TrialCouponRevision
	found := false
	for _, x := range cs {
		if x.CouponCode == code && (!found || x.RevisionNo > out.RevisionNo) {
			out = x
			found = true
		}
	}
	if !found {
		return out, domain.ErrNotFound
	}
	return out, nil
}
func (r *Repository) Coupons(id string) ([]domain.TrialCouponRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := []domain.TrialCouponRevision{}
	for _, x := range r.s.Coupons {
		if x.CaseID == id {
			o = append(o, x)
		}
	}
	sort.Slice(o, func(i, j int) bool {
		if o[i].CouponCode == o[j].CouponCode {
			return o[i].RevisionNo < o[j].RevisionNo
		}
		return o[i].CouponCode < o[j].CouponCode
	})
	return o, nil
}
func (r *Repository) SaveAssessment(a domain.AssessmentSnapshot) (domain.AssessmentSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, e := json.Marshal(a); e == nil {
		var cp domain.AssessmentSnapshot
		if json.Unmarshal(b, &cp) == nil {
			a = cp
		}
	}
	for i := range a.Findings {
		a.Findings[i].AssessmentID = a.ID
		a.Findings[i].ID = a.ID + "-" + strconv.Itoa(i+1)
	}
	r.s.Assessments[a.ID] = a
	r.addEvent(a.CaseID, "assessment.created", a.InputDigest)
	r.persist()
	return cloneAssessment(a), nil
}
func cloneAssessment(a domain.AssessmentSnapshot) domain.AssessmentSnapshot {
	return a
}
func (r *Repository) LatestAssessment(id string) (domain.AssessmentSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out domain.AssessmentSnapshot
	found := false
	for _, x := range r.s.Assessments {
		if x.CaseID == id && (!found || x.CreatedAt.After(out.CreatedAt) || (x.CreatedAt.Equal(out.CreatedAt) && x.ID > out.ID)) {
			out = x
			found = true
		}
	}
	if !found {
		return out, domain.ErrNotFound
	}
	return cloneAssessment(out), nil
}
func (r *Repository) Assessment(id string) (domain.AssessmentSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.s.Assessments[id]
	if !ok {
		return domain.AssessmentSnapshot{}, domain.ErrNotFound
	}
	return cloneAssessment(a), nil
}
func (r *Repository) CloseRisks(id, evidence string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.latestAssessmentLocked(id)
	if !ok {
		return domain.ErrNotFound
	}
	for i := range a.Findings {
		if a.Findings[i].Severity == "blocking" {
			a.Findings[i].Status = "closed"
			a.Findings[i].ResolutionEvidence = evidence
			t := time.Now()
			a.Findings[i].ResolvedAt = &t
		}
	}
	r.s.Assessments[a.ID] = a
	r.addEvent(id, "risks.closed", evidence)
	r.persist()
	return nil
}
func (r *Repository) CloseRisk(id, riskID, evidence string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.latestAssessmentLocked(id)
	if !ok {
		return domain.ErrNotFound
	}
	found := false
	for i := range a.Findings {
		if a.Findings[i].ID == riskID {
			found = true
			if a.Findings[i].Severity != "blocking" {
				return fmt.Errorf("%w: 仅可整改阻断风险", domain.ErrInvalid)
			}
			if a.Findings[i].Status == "closed" {
				return fmt.Errorf("%w: 风险已关闭", domain.ErrInvalid)
			}
			a.Findings[i].Status = "closed"
			a.Findings[i].ResolutionEvidence = mergeEvidence(a.Findings[i].ResolutionEvidence, evidence)
			t := time.Now()
			a.Findings[i].ResolvedAt = &t
		}
	}
	if !found {
		return fmt.Errorf("%w: 风险不存在", domain.ErrInvalid)
	}
	r.s.Assessments[a.ID] = a
	r.addEvent(id, "risk.closed", riskID+": "+evidence)
	r.persist()
	return nil
}
func (r *Repository) SaveReview(v domain.ReviewDecision) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.Reviews[v.ID] = v
	r.addEvent(v.CaseID, "review."+v.Decision, v.Reason)
	r.persist()
	return nil
}
func (r *Repository) SavePermit(p domain.WorkPermit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.Permits[p.ID] = p
	r.addEvent(p.CaseID, "permit.issued", p.PermitCode)
	r.persist()
	return nil
}
func (r *Repository) LatestReview(id string) (domain.ReviewDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out domain.ReviewDecision
	found := false
	for _, v := range r.s.Reviews {
		if v.CaseID == id && (!found || v.CreatedAt.After(out.CreatedAt) || (v.CreatedAt.Equal(out.CreatedAt) && v.ID > out.ID)) {
			out = v
			found = true
		}
	}
	if !found {
		return out, domain.ErrNotFound
	}
	return out, nil
}
func (r *Repository) Permit(id string) (domain.WorkPermit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.s.Permits {
		if p.CaseID == id {
			return p, nil
		}
	}
	return domain.WorkPermit{}, domain.ErrNotFound
}
func (r *Repository) Events(id string) ([]domain.AuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := []domain.AuditEvent{}
	for _, e := range r.s.Events {
		if e.CaseID == id {
			o = append(o, e)
		}
	}
	domain.SortEvents(o)
	return o, nil
}
func (r *Repository) addEvent(caseID, typ, detail string) {
	r.s.Events = append(r.s.Events, domain.AuditEvent{ID: "evt-" + time.Now().Format("20060102150405.000000000"), CaseID: caseID, Type: typ, Detail: detail, At: time.Now()})
}
func (r *Repository) saveKey(k string, v any) {
	if k != "" {
		b, _ := json.Marshal(v)
		r.s.Idempotency[k] = string(b)
	}
}
func (r *Repository) idempotent(k string) (string, bool) { v, ok := r.s.Idempotency[k]; return v, ok }
func (r *Repository) latestAssessmentLocked(id string) (domain.AssessmentSnapshot, bool) {
	var o domain.AssessmentSnapshot
	ok := false
	for _, a := range r.s.Assessments {
		if a.CaseID == id && (!ok || a.CreatedAt.After(o.CreatedAt) || (a.CreatedAt.Equal(o.CreatedAt) && a.ID > o.ID)) {
			o = a
			ok = true
		}
	}
	return o, ok
}
func mergeEvidence(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "；" + b
}
func (r *Repository) ResetForTests() { r.mu.Lock(); defer r.mu.Unlock(); r.init(); r.persist() }
func RemoveDB(path string)           { _ = os.Remove(path) }
