package store

import (
	"benzhiguji/internal/domain"
	"sort"
	"strings"
	"time"
)

type CaseBundle struct {
	Case       domain.RestorationCase
	Regions    []domain.DamageRegion
	Plan       domain.TreatmentPlanRevision
	Coupons    []domain.TrialCouponRevision
	Assessment domain.AssessmentSnapshot
	Events     []domain.AuditEvent
	Permit     domain.WorkPermit
}
type CaseFilter struct {
	Status                  domain.CaseStatus
	Owner, Collection, Text string
	From, To                time.Time
	WarningWindow           int
}
type CaseListItem struct {
	domain.RestorationCase
	Warning       domain.WarningLevel `json:"warning"`
	WarningLevel  domain.WarningLevel `json:"warningLevel"`
	RemainingDays int                 `json:"remainingDays"`
	StatusLabel   string              `json:"statusLabel"`
	StatusValue   domain.CaseStatus   `json:"status"`
	VersionValue  int                 `json:"version"`
}

func (r *Repository) Bundle(id string) (CaseBundle, error) {
	c, e := r.GetCase(id)
	if e != nil {
		return CaseBundle{}, e
	}
	rs, _ := r.Regions(id)
	p, _ := r.LatestPlan(id)
	cs, _ := r.Coupons(id)
	a, _ := r.LatestAssessment(id)
	ev, _ := r.Events(id)
	pr, _ := r.Permit(id)
	return CaseBundle{c, rs, p, cs, a, ev, pr}, nil
}
func (r *Repository) ListCases() []domain.RestorationCase {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := make([]domain.RestorationCase, 0, len(r.s.Cases))
	for _, c := range r.s.Cases {
		o = append(o, c)
	}
	sort.Slice(o, func(i, j int) bool {
		if o[i].TargetDate == o[j].TargetDate {
			if o[i].UpdatedAt.Equal(o[j].UpdatedAt) {
				return o[i].ID < o[j].ID
			}
			return o[i].UpdatedAt.After(o[j].UpdatedAt)
		}
		return o[i].TargetDate < o[j].TargetDate
	})
	return o
}
func (r *Repository) FilterCases(f CaseFilter) []CaseListItem {
	now := time.Now()
	out := []CaseListItem{}
	for _, c := range r.ListCases() {
		if f.Text != "" && !strings.Contains(strings.ToLower(c.ID+" "+c.Title+" "+c.CollectionCode), strings.ToLower(f.Text)) {
			continue
		}
		if f.Status != "" && c.Status != f.Status || f.Owner != "" && c.OwnerName != f.Owner || f.Collection != "" && !strings.Contains(strings.ToLower(c.CollectionCode), strings.ToLower(f.Collection)) {
			continue
		}
		t, _ := time.Parse("2006-01-02", c.TargetDate)
		if !f.From.IsZero() && t.Before(f.From) || !f.To.IsZero() && t.After(f.To) {
			continue
		}
		w, d := domain.DateWarning(c.TargetDate, now, f.WarningWindow)
		if c.Status == domain.StatusFrozen {
			w = domain.WarningNone
		}
		out = append(out, CaseListItem{RestorationCase: c, Warning: w, WarningLevel: w, RemainingDays: d, StatusLabel: domain.StatusLabel(c.Status), StatusValue: c.Status, VersionValue: c.Version})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TargetDate == out[j].TargetDate {
			if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
				return out[i].ID < out[j].ID
			}
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].TargetDate < out[j].TargetDate
	})
	return out
}
func (r *Repository) FindByStatus(status domain.CaseStatus) []domain.RestorationCase {
	all := r.ListCases()
	o := []domain.RestorationCase{}
	for _, c := range all {
		if c.Status == status {
			o = append(o, c)
		}
	}
	return o
}
func (r *Repository) SearchCases(q string) []domain.RestorationCase {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return r.ListCases()
	}
	o := []domain.RestorationCase{}
	for _, c := range r.ListCases() {
		if strings.Contains(strings.ToLower(c.ID), q) || strings.Contains(strings.ToLower(c.Title), q) || strings.Contains(strings.ToLower(c.CollectionCode), q) {
			o = append(o, c)
		}
	}
	return o
}
func (r *Repository) LastEvent(id string) (domain.AuditEvent, error) {
	ev, _ := r.Events(id)
	if len(ev) == 0 {
		return domain.AuditEvent{}, domain.ErrNotFound
	}
	return ev[len(ev)-1], nil
}
func (r *Repository) EventCount(id string) int { ev, _ := r.Events(id); return len(ev) }
func (r *Repository) HasPermit(id string) bool { _, e := r.Permit(id); return e == nil }
func (r *Repository) Touch(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.s.Cases[id]
	if !ok {
		return domain.ErrNotFound
	}
	if c.Status == domain.StatusFrozen {
		return domain.ErrFrozen
	}
	c.UpdatedAt = time.Now()
	r.s.Cases[id] = c
	r.persist()
	return nil
}
