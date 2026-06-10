package billingcalc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

func paramFloat(params map[string]any, key string, fallback float64) float64 {
	v, ok := params[key]
	if !ok || v == nil {
		return fallback
	}
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case uint:
		return float64(t)
	case uint64:
		return float64(t)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return f
		}
	}
	return fallback
}

func paramString(params map[string]any, key, fallback string) string {
	v, ok := params[key]
	if !ok || v == nil {
		return fallback
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return fallback
	}
	return s
}

func paramBool(params map[string]any, key string, fallback bool) bool {
	v, ok := params[key]
	if !ok || v == nil {
		return fallback
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(t)); err == nil {
			return b
		}
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return fallback
}

func jsonFloat(body []byte, fallback float64, paths ...string) float64 {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		r := gjson.GetBytes(body, path)
		if !r.Exists() {
			continue
		}
		if r.Type == gjson.Number {
			return r.Float()
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(r.String()), 64); err == nil {
			return f
		}
	}
	return fallback
}

func jsonString(body []byte, fallback string, paths ...string) string {
	for _, path := range paths {
		r := gjson.GetBytes(body, path)
		if !r.Exists() {
			continue
		}
		s := strings.TrimSpace(r.String())
		if s != "" {
			return s
		}
	}
	return fallback
}

func jsonBool(body []byte, fallback bool, paths ...string) bool {
	for _, path := range paths {
		r := gjson.GetBytes(body, path)
		if !r.Exists() {
			continue
		}
		if r.Type == gjson.True {
			return true
		}
		if r.Type == gjson.False {
			return false
		}
		if b, err := strconv.ParseBool(strings.TrimSpace(r.String())); err == nil {
			return b
		}
	}
	return fallback
}

func jsonArrayLen(body []byte, paths ...string) int {
	for _, path := range paths {
		r := gjson.GetBytes(body, path)
		if r.Exists() && r.IsArray() {
			return len(r.Array())
		}
	}
	return 0
}

func hasAny(body []byte, paths ...string) bool {
	for _, path := range paths {
		r := gjson.GetBytes(body, path)
		if r.Exists() {
			return true
		}
	}
	return false
}
