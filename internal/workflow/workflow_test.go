package workflow

import (
	"benzhiguji/internal/domain"
	"benzhiguji/internal/store"
	"testing"
)

func newFlow(t *testing.T) *Service {
	r, e := store.Open(t.TempDir() + "/case.json")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	return New(r)
}
func TestCompletePermitFlow(t *testing.T) {
	s := newFlow(t)
	c, e := s.CreateCase(domain.RestorationCase{CollectionCode: "C", Title: "古籍", MaterialProfile: "纸本", OwnerName: "甲", TargetDate: "2030-01-01"}, "k")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.AddRegion(domain.DamageRegion{CaseID: c.ID, RegionCode: "R1", Location: "卷首", DamageType: "虫蛀", Severity: "中", WidthMM: 2, HeightMM: 3}, c.Version, "")
	if e != nil {
		t.Fatal(e)
	}
	_ = r
	c, _ = s.Repo.GetCase(c.ID)
	p, e := s.AddPlan(domain.TreatmentPlanRevision{CaseID: c.ID, MaterialLots: []string{"P"}, ProcedureSteps: []string{"清洁"}, RegionBindings: []string{"R1"}}, c.Version, "")
	if e != nil {
		t.Fatal(e)
	}
	_ = p
	c, _ = s.Repo.GetCase(c.ID)
	_, e = s.AddCoupon(domain.TrialCouponRevision{CaseID: c.ID, CouponCode: "T1", Substrate: "纸", Formula: "1:1", Environment: "20C", ObservationHours: 48, PHValue: 7, PeelStrength: 2, ColorDelta: 1, ReversibilityGrade: "好"}, c.Version)
	if e != nil {
		t.Fatal(e)
	}
	c, _ = s.Repo.GetCase(c.ID)
	_, e = s.Evaluate(c.ID, c.Version)
	if e != nil {
		t.Fatal(e)
	}
	c, _ = s.Repo.GetCase(c.ID)
	if _, e = s.SubmitReview(c.ID, c.Version, "复核员"); e != nil {
		t.Fatal(e)
	}
	c, _ = s.Repo.GetCase(c.ID)
	permit, e := s.Decide(c.ID, c.Version, "复核员", "approve", "")
	if e != nil {
		t.Fatal(e)
	}
	if permit.VerificationStatus != "valid" {
		t.Fatal("许可未生效")
	}
}
func TestStaleVersionRejected(t *testing.T) {
	s := newFlow(t)
	c, _ := s.CreateCase(domain.RestorationCase{CollectionCode: "C", Title: "古籍", MaterialProfile: "纸本", OwnerName: "甲", TargetDate: "2030-01-01"}, "")
	_, e := s.AddRegion(domain.DamageRegion{CaseID: c.ID, RegionCode: "R1", Location: "卷首", DamageType: "虫蛀", Severity: "中", WidthMM: 2, HeightMM: 3}, c.Version, "")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.AddRegion(domain.DamageRegion{CaseID: c.ID, RegionCode: "R2", Location: "卷尾", DamageType: "水渍", Severity: "低", WidthMM: 2, HeightMM: 3}, c.Version, "")
	if e != domain.ErrConflict {
		t.Fatalf("期望版本冲突，得到 %v", e)
	}
}
func TestBlockingRiskNeedsEvidence(t *testing.T) {
	s := newFlow(t)
	c, _ := s.CreateCase(domain.RestorationCase{CollectionCode: "C", Title: "古籍", MaterialProfile: "纸本", OwnerName: "甲", TargetDate: "2030-01-01"}, "")
	_, _ = s.AddRegion(domain.DamageRegion{CaseID: c.ID, RegionCode: "R1", Location: "卷首", DamageType: "虫蛀", Severity: "中", WidthMM: 2, HeightMM: 3}, c.Version, "")
	c, _ = s.Repo.GetCase(c.ID)
	_, _ = s.AddPlan(domain.TreatmentPlanRevision{CaseID: c.ID, MaterialLots: []string{"P"}, ProcedureSteps: []string{"清洁"}, RegionBindings: []string{"R1"}}, c.Version, "")
	c, _ = s.Repo.GetCase(c.ID)
	_, _ = s.AddCoupon(domain.TrialCouponRevision{CaseID: c.ID, CouponCode: "T1", Substrate: "纸", Formula: "1:1", Environment: "20C", ObservationHours: 1, PHValue: 3, ReversibilityGrade: "差"}, c.Version)
	c, _ = s.Repo.GetCase(c.ID)
	_, e := s.Evaluate(c.ID, c.Version)
	if e != nil {
		t.Fatal(e)
	}
	c, _ = s.Repo.GetCase(c.ID)
	if _, e = s.SubmitReview(c.ID, c.Version, "复核员"); e == nil {
		t.Fatal("阻断风险不应直接进入复核")
	}
}
