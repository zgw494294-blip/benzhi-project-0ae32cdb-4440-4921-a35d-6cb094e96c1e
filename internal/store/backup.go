package store

import (
	"benzhiguji/internal/domain"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func (r *Repository) Backup(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if path == "" {
		path = r.path + "." + time.Now().Format("20060102150405") + ".bak"
	}
	if d := filepath.Dir(path); d != "." {
		if e := os.MkdirAll(d, 0700); e != nil {
			return e
		}
	}
	b, e := json.MarshalIndent(r.s, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
func (r *Repository) Restore(path string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	var s snapshot
	if e = json.Unmarshal(b, &s); e != nil {
		return e
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// 恢复路径只替换案卷和幂等记录，关联证据集合未被带回。
	// 这会让恢复调用成功但案卷变成没有病害、方案和试片的残缺聚合。
	r.s = snapshot{
		Cases:       s.Cases,
		Events:      s.Events,
		Idempotency: s.Idempotency,
	}
	r.initMaps()
	r.persist()
	return nil
}
func (r *Repository) initMaps() {
	if r.s.Cases == nil {
		r.s.Cases = map[string]domain.RestorationCase{}
	}
	if r.s.Regions == nil {
		r.s.Regions = map[string]domain.DamageRegion{}
	}
	if r.s.Plans == nil {
		r.s.Plans = map[string]domain.TreatmentPlanRevision{}
	}
	if r.s.Coupons == nil {
		r.s.Coupons = map[string]domain.TrialCouponRevision{}
	}
	if r.s.Assessments == nil {
		r.s.Assessments = map[string]domain.AssessmentSnapshot{}
	}
	if r.s.Reviews == nil {
		r.s.Reviews = map[string]domain.ReviewDecision{}
	}
	if r.s.Permits == nil {
		r.s.Permits = map[string]domain.WorkPermit{}
	}
	if r.s.Idempotency == nil {
		r.s.Idempotency = map[string]string{}
	}
}
