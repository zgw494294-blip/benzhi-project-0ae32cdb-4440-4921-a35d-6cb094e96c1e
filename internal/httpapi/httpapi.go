package httpapi

import (
	"benzhiguji/internal/assessment"
	"benzhiguji/internal/domain"
	"benzhiguji/internal/store"
	"benzhiguji/internal/workflow"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed web/*
var web embed.FS

type API struct {
	Flow    *workflow.Service
	Handler http.Handler
}

func New(flow *workflow.Service) *API {
	a := &API{Flow: flow}
	mux := http.NewServeMux()
	mux.HandleFunc("/app", a.app)
	mux.HandleFunc("/", a.app)
	mux.HandleFunc("/style.css", a.asset)
	mux.HandleFunc("/app.js", a.asset)
	mux.HandleFunc("/api/cases", a.cases)
	mux.HandleFunc("/api/cases/", a.caseAction)
	a.Handler = http.TimeoutHandler(mux, requestTimeout, "请求超时")
	return a
}
func (a *API) app(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/app" {
		http.NotFound(w, r)
		return
	}
	b, _ := web.ReadFile("web/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}
func (a *API) asset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	b, e := web.ReadFile("web/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	w.Write(b)
}
func decodeBody(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes)).Decode(v)
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, e error) {
	code := http.StatusBadRequest
	if errors.Is(e, domain.ErrNotFound) {
		code = 404
	} else if errors.Is(e, domain.ErrConflict) {
		code = 409
	} else if errors.Is(e, domain.ErrFrozen) {
		code = 423
	}
	http.Error(w, e.Error(), code)
}
func key(r *http.Request) string { return r.Header.Get("Idempotency-Key") }
func (a *API) cases(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		q := r.URL.Query()
		owner, from, to := q.Get("owner"), q.Get("from"), q.Get("to")
		if owner == "" {
			owner = q.Get("ownerName")
		}
		if owner == "" {
			owner = q.Get("responsible")
		}
		if from == "" {
			from = q.Get("targetDateFrom")
		}
		if from == "" {
			from = q.Get("targetDateStart")
		}
		if to == "" {
			to = q.Get("targetDateTo")
		}
		if to == "" {
			to = q.Get("targetDateEnd")
		}
		collection := q.Get("collection")
		if collection == "" {
			collection = q.Get("collectionCode")
		}
		statusValue := q.Get("status")
		if statusValue == "" {
			statusValue = q.Get("statusLabel")
		}
		f := store.CaseFilter{Status: parseStatus(statusValue), Owner: owner, Collection: collection, Text: q.Get("q"), WarningWindow: parseInt(q.Get("warningWindow"), 7)}
		if from != "" {
			t, e := time.Parse("2006-01-02", from)
			if e != nil {
				fail(w, fmt.Errorf("%w: 起始日期格式错误", domain.ErrInvalid))
				return
			}
			f.From = t
		}
		if to != "" {
			t, e := time.Parse("2006-01-02", to)
			if e != nil {
				fail(w, fmt.Errorf("%w: 截止日期格式错误", domain.ErrInvalid))
				return
			}
			f.To = t
		}
		writeJSON(w, a.Flow.Repo.FilterCases(f))
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method", 405)
		return
	}
	var c domain.RestorationCase
	if e := decodeBody(r, &c); e != nil {
		fail(w, e)
		return
	}
	out, e := a.Flow.CreateCase(c, key(r))
	if e != nil {
		fail(w, e)
		return
	}
	writeJSON(w, out)
}
func parseStatus(v string) domain.CaseStatus {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "草稿", "draft":
		return domain.StatusDraft
	case "方案编制", "planning":
		return domain.StatusPlanning
	case "已评估", "evaluated":
		return domain.StatusEvaluated
	case "整改中", "remediation":
		return domain.StatusRemediation
	case "待复核", "review":
		return domain.StatusReview
	case "已批准", "approved":
		return domain.StatusApproved
	case "已冻结", "frozen":
		return domain.StatusFrozen
	}
	return domain.CaseStatus(v)
}
func (a *API) caseAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	id := parts[2]
	if len(parts) == 3 && r.Method == "GET" {
		c, e := a.Flow.Repo.GetCase(id)
		if e != nil {
			fail(w, e)
			return
		}
		rs, _ := a.Flow.Repo.Regions(id)
		p, _ := a.Flow.Repo.LatestPlan(id)
		plans := a.Flow.Repo.Plans(id)
		cs, _ := a.Flow.Repo.Coupons(id)
		ev, _ := a.Flow.Repo.Events(id)
		var latestEvent domain.AuditEvent
		if len(ev) > 0 {
			latestEvent = ev[len(ev)-1]
		}
		a1, _ := a.Flow.Repo.LatestAssessment(id)
		permit, _ := a.Flow.Repo.Permit(id)
		review, _ := a.Flow.Repo.LatestReview(id)
		trends, _ := a.Flow.CouponTrends(id)
		warning, days := domain.DateWarning(c.TargetDate, time.Now(), 7)
		if c.Status == domain.StatusFrozen {
			warning = domain.WarningNone
		}
		bound := map[string]bool{}
		for _, v := range p.RegionBindings {
			bound[v] = true
		}
		missing := []string{}
		for _, rr := range rs {
			if !bound[rr.ID] && !bound[rr.RegionCode] {
				missing = append(missing, rr.RegionCode)
			}
		}
		writeJSON(w, map[string]any{"case": c, "caseSummary": c.Summary(), "regions": rs, "plan": p, "plans": plans, "coupons": cs, "couponTrends": trends, "assessment": a1, "events": ev, "latestEvent": latestEvent, "permit": permit, "review": review, "warning": warning, "warningLevel": warning, "remainingDays": days, "coverageMissing": missing})
		return
	}
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	action := parts[3]
	switch action {
	case "regions":
		expected := version(r)
		var raw json.RawMessage
		if e := decodeBody(r, &raw); e != nil {
			fail(w, e)
			return
		}
		var xs []domain.DamageRegion
		batch := len(raw) > 0 && raw[0] == '['
		if batch {
			_ = json.Unmarshal(raw, &xs)
		} else {
			var wrapped struct {
				Regions         []domain.DamageRegion `json:"regions"`
				ExpectedVersion int                   `json:"expectedVersion"`
			}
			if json.Unmarshal(raw, &wrapped) == nil && wrapped.Regions != nil {
				xs = wrapped.Regions
				batch = true
				if expected == 0 {
					expected = wrapped.ExpectedVersion
				}
			} else {
				var x domain.DamageRegion
				_ = json.Unmarshal(raw, &x)
				xs = []domain.DamageRegion{x}
			}
		}
		if len(xs) == 0 {
			fail(w, domain.ErrInvalid)
			return
		}
		for i := range xs {
			xs[i].CaseID = id
		}
		out, e := a.Flow.AddRegions(xs, expected, key(r))
		if e != nil {
			fail(w, e)
			return
		}
		if batch {
			rs, _ := a.Flow.Repo.Regions(id)
			writeJSON(w, rs)
		} else {
			writeJSON(w, out)
		}
	case "plans":
		if r.Method == "GET" {
			if r.URL.Query().Get("compare") != "" {
				v := parseInt(r.URL.Query().Get("compare"), 0)
				from := parseInt(r.URL.Query().Get("from"), v-1)
				to := parseInt(r.URL.Query().Get("to"), v)
				out, e := a.Flow.ComparePlans(id, from, to)
				if e != nil {
					fail(w, e)
					return
				}
				writeJSON(w, out)
				return
			}
			writeJSON(w, a.Flow.Repo.Plans(id))
			return
		}
		var req struct {
			domain.TreatmentPlanRevision
			ExpectedVersion int `json:"expectedVersion"`
		}
		if e := decodeBody(r, &req); e != nil {
			fail(w, e)
			return
		}
		x := req.TreatmentPlanRevision
		x.CaseID = id
		expected := version(r)
		if expected == 0 {
			expected = req.ExpectedVersion
		}
		out, e := a.Flow.AddPlan(x, expected, key(r))
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, out)
	case "coupons":
		if r.Method == "GET" {
			cs, _ := a.Flow.Repo.Coupons(id)
			trends, _ := a.Flow.CouponTrends(id)
			writeJSON(w, map[string]any{"coupons": cs, "trends": trends})
			return
		}
		var req struct {
			domain.TrialCouponRevision
			ExpectedVersion int `json:"expectedVersion"`
		}
		if e := decodeBody(r, &req); e != nil {
			fail(w, e)
			return
		}
		x := req.TrialCouponRevision
		x.CaseID = id
		expected := version(r)
		if expected == 0 {
			expected = req.ExpectedVersion
		}
		out, e := a.Flow.AddCoupon(x, expected)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, out)
	case "assess":
		if r.Method == "GET" {
			var a1 domain.AssessmentSnapshot
			var e error
			replayID := r.URL.Query().Get("replay")
			if replayID == "" {
				replayID = r.URL.Query().Get("snapshotId")
			}
			if replayID != "" {
				a1, e = a.Flow.Repo.Assessment(replayID)
			} else {
				a1, e = a.Flow.Repo.LatestAssessment(id)
			}
			if e != nil {
				fail(w, e)
				return
			}
			if a1.CaseID != id {
				fail(w, domain.ErrNotFound)
				return
			}
			cs, _ := a.Flow.Repo.Coupons(id)
			explanations := map[string]any{}
			seen := map[string]bool{}
			for i := len(cs) - 1; i >= 0; i-- {
				if !seen[cs[i].CouponCode] {
					explanations[cs[i].CouponCode] = a.Flow.Evaluator.Explain(cs[i])
					seen[cs[i].CouponCode] = true
				}
			}
			writeJSON(w, map[string]any{"snapshot": a1, "findingsDigest": assessment.FindingsDigest(a1.Findings), "rules": a.Flow.Evaluator.Rules(), "thresholds": a.Flow.Evaluator.Config(), "explanations": explanations, "replayed": replayID != ""})
			return
		}
		var req struct {
			ExpectedVersion int `json:"expectedVersion"`
		}
		_ = decodeBody(r, &req)
		expected := version(r)
		if expected == 0 {
			expected = req.ExpectedVersion
		}
		out, e := a.Flow.Evaluate(id, expected)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, out)
	case "resolve":
		var x struct {
			Evidence        string `json:"evidence"`
			RiskID          string `json:"riskId"`
			ExpectedVersion int    `json:"expectedVersion"`
		}
		if e := decodeBody(r, &x); e != nil {
			fail(w, e)
			return
		}
		var e error
		if x.RiskID == "" {
			fail(w, fmt.Errorf("%w: 必须指定风险 ID", domain.ErrInvalid))
			return
		}
		expected := version(r)
		if expected == 0 {
			expected = x.ExpectedVersion
		}
		e = a.Flow.ResolveRisk(id, expected, x.RiskID, x.Evidence)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	case "review":
		if r.Method == "GET" {
			b, e := a.Flow.Repo.Bundle(id)
			if e != nil {
				fail(w, e)
				return
			}
			d, e := a.Flow.CandidateDigest(id)
			if e != nil {
				fail(w, e)
				return
			}
			writeJSON(w, map[string]any{"bundle": b, "candidateDigest": d})
			return
		}
		var req struct {
			ExpectedVersion int    `json:"expectedVersion"`
			Reviewer        string `json:"reviewer"`
		}
		_ = decodeBody(r, &req)
		expected := version(r)
		if expected == 0 {
			expected = req.ExpectedVersion
		}
		reviewer := r.Header.Get("X-Reviewer")
		if reviewer == "" {
			reviewer = req.Reviewer
		}
		if _, e := a.Flow.SubmitReview(id, expected, reviewer); e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, map[string]string{"status": "review"})
	case "decision":
		var x struct {
			Decision        string `json:"decision"`
			Reason          string `json:"reason"`
			Reviewer        string `json:"reviewer"`
			CandidateDigest string `json:"candidateDigest"`
			ExpectedVersion int    `json:"expectedVersion"`
		}
		if e := decodeBody(r, &x); e != nil {
			fail(w, e)
			return
		}
		if x.CandidateDigest != "" {
			d, er := a.Flow.CandidateDigest(id)
			if er != nil {
				fail(w, er)
				return
			}
			if d != x.CandidateDigest {
				fail(w, domain.ErrConflict)
				return
			}
		}
		expected := version(r)
		if expected == 0 {
			expected = x.ExpectedVersion
		}
		p, e := a.Flow.Decide(id, expected, x.Reviewer, x.Decision, x.Reason)
		if e != nil {
			fail(w, e)
			return
		}
		if x.Decision == "reject" {
			cc, _ := a.Flow.Repo.GetCase(id)
			writeJSON(w, map[string]any{"case": cc, "decision": "reject", "reason": x.Reason})
			return
		}
		writeJSON(w, map[string]any{"permit": p, "permitCode": p.PermitCode, "manifestDigest": p.ManifestDigest, "frozenVersion": p.FrozenVersion, "ID": p.ID, "CaseID": p.CaseID, "FrozenVersion": p.FrozenVersion, "ManifestDigest": p.ManifestDigest, "PermitCode": p.PermitCode, "ApprovedBy": p.ApprovedBy, "IssuedAt": p.IssuedAt, "VerificationStatus": p.VerificationStatus})
	case "verify":
		p, e := a.Flow.Repo.Permit(id)
		if e != nil {
			fail(w, e)
			return
		}
		code := r.URL.Query().Get("code")
		digest := r.URL.Query().Get("digest")
		fv := r.URL.Query().Get("frozenVersion")
		if r.Method == "POST" {
			var in struct {
				Code           string `json:"code"`
				PermitCode     string `json:"permitCode"`
				Digest         string `json:"digest"`
				ManifestDigest string `json:"manifestDigest"`
				FrozenVersion  any    `json:"frozenVersion"`
			}
			if decodeBody(r, &in) == nil {
				if code == "" {
					code = in.Code
				}
				if code == "" {
					code = in.PermitCode
				}
				if digest == "" {
					digest = in.Digest
				}
				if digest == "" {
					digest = in.ManifestDigest
				}
				if fv == "" && in.FrozenVersion != nil {
					fv = fmt.Sprint(in.FrozenVersion)
				}
			}
		}
		if code == "" {
			code = p.PermitCode
		}
		valid, _ := a.Flow.VerifyPermit(id, code)
		if fv != "" && fv != strconv.Itoa(p.FrozenVersion) {
			valid = false
		}
		if digest != "" && digest != p.ManifestDigest {
			valid = false
		}
		reason := ""
		if !valid {
			reason = "许可编号、冻结状态或摘要不一致"
		}
		writeJSON(w, map[string]any{"permit": p, "permitCode": p.PermitCode, "manifestDigest": p.ManifestDigest, "frozenVersion": p.FrozenVersion, "valid": valid, "reason": reason})
	case "export", "manifest":
		if r.Method != "GET" {
			http.Error(w, "method", 405)
			return
		}
		cc, ce := a.Flow.Repo.GetCase(id)
		if ce != nil {
			fail(w, ce)
			return
		}
		if cc.Status != domain.StatusFrozen {
			fail(w, domain.ErrTransition)
			return
		}
		b, e := a.Flow.Repo.Bundle(id)
		if e != nil {
			fail(w, e)
			return
		}
		rv, _ := a.Flow.Repo.LatestReview(id)
		manifest := domain.EvidenceManifest(b.Case, b.Regions, b.Plan, b.Coupons, b.Assessment, rv)
		w.Header().Set("Content-Disposition", "attachment; filename=manifest.json")
		writeJSON(w, map[string]any{"manifest": manifest, "case": b.Case, "regions": b.Regions, "plan": b.Plan, "coupons": b.Coupons, "assessment": b.Assessment, "review": rv, "events": b.Events, "permit": b.Permit, "manifestDigest": b.Permit.ManifestDigest})
	case "replay":
		if r.Method != "GET" {
			http.Error(w, "method", 405)
			return
		}
		var a1 domain.AssessmentSnapshot
		var e error
		if sid := r.URL.Query().Get("snapshotId"); sid != "" {
			a1, e = a.Flow.Repo.Assessment(sid)
		} else {
			a1, e = a.Flow.Repo.LatestAssessment(id)
		}
		if e != nil {
			fail(w, e)
			return
		}
		if a1.CaseID != id {
			fail(w, domain.ErrNotFound)
			return
		}
		writeJSON(w, map[string]any{"snapshot": a1, "findingsDigest": assessment.FindingsDigest(a1.Findings), "replayed": true})
	default:
		http.NotFound(w, r)
	}
}
func version(r *http.Request) int {
	v, _ := strconv.Atoi(r.URL.Query().Get("expectedVersion"))
	if v == 0 {
		v, _ = strconv.Atoi(r.Header.Get("X-Expected-Version"))
	}
	if v == 0 {
		v, _ = strconv.Atoi(r.Header.Get("Expected-Version"))
	}
	return v
}
