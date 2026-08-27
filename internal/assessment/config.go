package assessment

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	MaxColorDelta  float64 `json:"maxColorDelta"`
	MinPH          float64 `json:"minPH"`
	MinPeel        float64 `json:"minPeel"`
	MinObservation int     `json:"minObservation"`
}

func (e Evaluator) Config() Config {
	return Config{e.MaxColorDelta, e.MinPH, e.MinPeel, e.MinObservation}
}
func FromConfig(c Config) Evaluator {
	e := DefaultEvaluator()
	if c.MaxColorDelta > 0 {
		e.MaxColorDelta = c.MaxColorDelta
	}
	if c.MinPH > 0 {
		e.MinPH = c.MinPH
	}
	if c.MinPeel > 0 {
		e.MinPeel = c.MinPeel
	}
	if c.MinObservation > 0 {
		e.MinObservation = c.MinObservation
	}
	return e
}
func LoadConfig(path string) (Evaluator, error) {
	if path == "" {
		return DefaultEvaluator(), nil
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return DefaultEvaluator(), e
	}
	var c Config
	if e = json.Unmarshal(b, &c); e != nil {
		return DefaultEvaluator(), fmt.Errorf("规则配置格式错误: %w", e)
	}
	if (c.MaxColorDelta != 0 && c.MaxColorDelta <= 0) || (c.MinPH != 0 && c.MinPH <= 0) || (c.MinPeel != 0 && c.MinPeel <= 0) || (c.MinObservation != 0 && c.MinObservation <= 0) {
		return DefaultEvaluator(), fmt.Errorf("规则配置阈值必须为正数，已回退默认值")
	}
	return FromConfig(c), nil
}
func SaveConfig(path string, e Evaluator) error {
	b, e2 := json.MarshalIndent(e.Config(), "", "  ")
	if e2 != nil {
		return e2
	}
	return os.WriteFile(path, b, 0600)
}
func Thresholds(e Evaluator) map[string]float64 {
	return map[string]float64{"maxColorDelta": e.MaxColorDelta, "minPH": e.MinPH, "minPeel": e.MinPeel, "minObservation": float64(e.MinObservation)}
}
