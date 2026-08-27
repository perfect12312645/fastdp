package utils

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var numberRegex = regexp.MustCompile(`^\s*([+-]?\d+\.?\d*)\s*%?\s*`)

// ParseNumber 从字符串中提取第一个数字
// 支持：42, 3.14, 42.5%, +10, -5.5
// 返回 (值, 是否成功)
func ParseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	// 取第一行
	if idx := strings.IndexAny(s, "\n\r"); idx >= 0 {
		s = s[:idx]
	}
	m := numberRegex.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ComputeAggregate 计算聚合结果
// 支持：avg, max, min, sum, median, p95, p99, stddev
func ComputeAggregate(values []float64, aggType string) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}

 	switch aggType {
	case "sum", "avg", "stddev":
		var sum float64
		for _, v := range values {
			sum += v
		}
		if aggType == "sum" {
			return sum, true
		}
		mean := sum / float64(len(values))
		if aggType == "avg" {
			return mean, true
		}
		// stddev
		var variance float64
		for _, v := range values {
			diff := v - mean
			variance += diff * diff
		}
		variance /= float64(len(values))
		return math.Sqrt(variance), true
	case "max":
		max := values[0]
		for _, v := range values[1:] {
			if v > max {
				max = v
			}
		}
		return max, true
	case "min":
		min := values[0]
		for _, v := range values[1:] {
			if v < min {
				min = v
			}
		}
		return min, true
	case "median", "p95", "p99":
		return computeStats(values, aggType)
	}
	return 0, false
}

// computeStats 计算中位数、百分位数、标准差
func computeStats(values []float64, aggType string) (float64, bool) {
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	n := len(sorted)

	switch aggType {
	case "median":
		if n%2 == 1 {
			return sorted[n/2], true
		}
		return (sorted[n/2-1] + sorted[n/2]) / 2, true
	case "p95":
		idx := int(math.Ceil(0.95*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return sorted[idx], true
	case "p99":
		idx := int(math.Ceil(0.99*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return sorted[idx], true
	}
	return 0, false
}
