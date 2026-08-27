package main

import (
	"benzhiguji/internal/httpapi"
	"benzhiguji/internal/store"
	"benzhiguji/internal/workflow"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19081", "监听地址")
	self := flag.Bool("selfcheck", false, "运行自检")
	dbPath := flag.String("db", "guji.db", "SQLite 文件")
	flag.Parse()
	addrProvided := false
	flag.CommandLine.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrProvided = true
		}
	})
	if env := os.Getenv("PORT"); env != "" && !addrProvided {
		if p, e := strconv.Atoi(env); e == nil && p > 0 && p < 65536 {
			*addr = "127.0.0.1:" + env
		}
	}
	if *addr == "" {
		*addr = "127.0.0.1:19081"
	}
	repo, e := store.Open(*dbPath)
	if e != nil {
		panic(e)
	}
	defer repo.Close()
	api := httpapi.New(workflow.New(repo))
	srv := &http.Server{Addr: *addr, Handler: api.Handler, ReadHeaderTimeout: 3 * time.Second}
	if *self {
		if e := runSelfcheck(srv, *addr); e != nil {
			panic(e)
		}
		return
	}
	ln, e := net.Listen("tcp", *addr)
	if e != nil {
		panic(e)
	}
	fmt.Println("古籍修复核验台监听", ln.Addr().String())
	if e = srv.Serve(ln); e != nil && e != http.ErrServerClosed {
		panic(e)
	}
}
func runSelfcheck(srv *http.Server, addr string) error {
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		return e
	}
	actual := ln.Addr().String()
	go srv.Serve(ln)
	defer func() {
		ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		srv.Shutdown(ctx)
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	post := func(path string, v string, headers map[string]string) ([]byte, error) {
		req, _ := http.NewRequest("POST", "http://"+actual+path, bytes.NewBufferString(v))
		req.Header.Set("Content-Type", "application/json")
		for k, x := range headers {
			req.Header.Set(k, x)
		}
		res, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 300 {
			return nil, fmt.Errorf("%s: %s", path, b)
		}
		return b, nil
	}
	b, e := post("/api/cases", `{"collectionCode":"SC-001","title":"自检古籍","materialProfile":"纸本","ownerName":"自检员","targetDate":"2030-01-01"}`, map[string]string{"Idempotency-Key": "self-case-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
	if e != nil {
		return e
	}
	var c struct {
		ID      string
		Version int
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return e
	}
	base := func() map[string]string { return map[string]string{"X-Expected-Version": strconv.Itoa(c.Version)} }
	b, e = post("/api/cases/"+c.ID+"/regions", `{"regionCode":"R1","location":"卷首","damageType":"虫蛀","severity":"中","widthMM":10,"heightMM":5}`, base())
	if e != nil {
		return e
	}
	_ = b // reload version
	res, e := client.Get("http://" + actual + "/api/cases/" + c.ID)
	if e != nil {
		return e
	}
	var d struct {
		Case    struct{ Version int }
		Regions []any
	}
	json.NewDecoder(res.Body).Decode(&d)
	res.Body.Close()
	c.Version = d.Case.Version
	b, e = post("/api/cases/"+c.ID+"/plans", `{"materialLots":["P-1"],"procedureSteps":["清洁"],"regionBindings":["R1"]}`, base())
	if e != nil {
		return e
	}
	_ = b
	res, _ = client.Get("http://" + actual + "/api/cases/" + c.ID)
	json.NewDecoder(res.Body).Decode(&d)
	res.Body.Close()
	c.Version = d.Case.Version
	b, e = post("/api/cases/"+c.ID+"/coupons", `{"couponCode":"C1","substrate":"宣纸","formula":"1:1","environment":"20C","observationHours":48,"colorDelta":1,"phValue":7,"peelStrength":2,"reversibilityGrade":"好"}`, base())
	if e != nil {
		return e
	}
	res, _ = client.Get("http://" + actual + "/api/cases/" + c.ID)
	json.NewDecoder(res.Body).Decode(&d)
	res.Body.Close()
	c.Version = d.Case.Version
	b, e = post("/api/cases/"+c.ID+"/assess", `{}`, base())
	if e != nil {
		return e
	}
	res, _ = client.Get("http://" + actual + "/api/cases/" + c.ID)
	json.NewDecoder(res.Body).Decode(&d)
	res.Body.Close()
	c.Version = d.Case.Version
	b, e = post("/api/cases/"+c.ID+"/review", `{}`, base())
	if e != nil {
		return e
	}
	res, _ = client.Get("http://" + actual + "/api/cases/" + c.ID)
	json.NewDecoder(res.Body).Decode(&d)
	res.Body.Close()
	c.Version = d.Case.Version
	b, e = post("/api/cases/"+c.ID+"/decision", `{"decision":"approve","reviewer":"复核员"}`, base())
	if e != nil {
		return e
	}
	var p struct{ PermitCode string }
	json.Unmarshal(b, &p)
	res, e = client.Get("http://" + actual + "/api/cases/" + c.ID + "/export")
	if e != nil {
		return e
	}
	exportBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode >= 300 || len(exportBody) == 0 {
		return fmt.Errorf("导出失败: %s", exportBody)
	}
	var exported struct {
		ManifestDigest string `json:"manifestDigest"`
		Permit         struct {
			FrozenVersion int `json:"FrozenVersion"`
		} `json:"permit"`
	}
	if json.Unmarshal(exportBody, &exported) != nil {
		return fmt.Errorf("导出格式错误")
	}
	verifyURL := "http://" + actual + "/api/cases/" + c.ID + "/verify?code=" + p.PermitCode + "&digest=" + exported.ManifestDigest + "&frozenVersion=" + strconv.Itoa(exported.Permit.FrozenVersion)
	res, e = client.Get(verifyURL)
	if e != nil {
		return e
	}
	var vr struct {
		Valid bool `json:"valid"`
	}
	e = json.NewDecoder(res.Body).Decode(&vr)
	res.Body.Close()
	if e != nil || !vr.Valid {
		return fmt.Errorf("验真失败")
	}
	fmt.Println("selfcheck passed", p.PermitCode)
	return nil
}
