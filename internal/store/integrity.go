package store

import (
	"benzhiguji/internal/domain"
	"fmt"
	"sort"
	"strings"
	"time"
)

type IntegrityReport struct {
	Cases, Regions, Plans, Coupons, Assessments, Permits, Events int
	Valid                                                        bool
	Issues                                                       []string
}

func (r *Repository) Integrity() IntegrityReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := IntegrityReport{Cases: len(r.s.Cases), Regions: len(r.s.Regions), Plans: len(r.s.Plans), Coupons: len(r.s.Coupons), Assessments: len(r.s.Assessments), Permits: len(r.s.Permits), Events: len(r.s.Events), Valid: true}
	for id, c := range r.s.Cases {
		if c.ID != id {
			x.Issues = append(x.Issues, "案卷键与 ID 不一致")
		}
		if c.Version < 1 {
			x.Issues = append(x.Issues, fmt.Sprintf("案卷 %s 版本无效", id))
		}
	}
	for id, p := range r.s.Permits {
		if _, ok := r.s.Cases[p.CaseID]; !ok {
			x.Issues = append(x.Issues, "许可关联案卷不存在")
		}
		if id == "" {
			x.Issues = append(x.Issues, "许可 ID 为空")
		}
	}
	x.Valid = len(x.Issues) == 0
	return x
}
func (r *Repository) ExportManifest(id string) (map[string]any, error) {
	b, e := r.Bundle(id)
	if e != nil {
		return nil, e
	}
	if b.Case.Status != domain.StatusFrozen {
		return nil, domain.ErrTransition
	}
	rv, _ := r.LatestReview(id)
	manifest := domain.EvidenceManifest(b.Case, b.Regions, b.Plan, b.Coupons, b.Assessment, rv)
	digest := domain.ManifestDigest(manifest)
	return map[string]any{"manifest": manifest, "manifestDigest": digest, "case": b.Case, "regions": b.Regions, "plan": b.Plan, "coupons": b.Coupons, "assessment": b.Assessment, "review": rv, "events": b.Events, "permit": b.Permit}, nil
}
func (r *Repository) AuditSince(id string, from time.Time) []domain.AuditEvent {
	ev, _ := r.Events(id)
	o := []domain.AuditEvent{}
	for _, x := range ev {
		if x.At.After(from) {
			o = append(o, x)
		}
	}
	return o
}
func (r *Repository) EventTypes(id string) []string {
	ev, _ := r.Events(id)
	seen := map[string]bool{}
	o := []string{}
	for _, x := range ev {
		if !seen[x.Type] {
			seen[x.Type] = true
			o = append(o, x.Type)
		}
	}
	sort.Strings(o)
	return o
}
func (r *Repository) ContainsDetail(id, q string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	q = strings.ToLower(q)
	for _, e := range r.s.Events {
		if e.CaseID == id && strings.Contains(strings.ToLower(e.Detail), q) {
			return true
		}
	}
	return false
}
