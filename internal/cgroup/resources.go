package cgroup

import (
	"fmt"
	"math"
	"strconv"
)

type Resources struct {
	CPUShares int64
	CPUQuota  float64
	MemoryMB  int64
	PidsLimit int64
}

func Validate(resources Resources) error {
	if resources.CPUShares < 0 {
		return fmt.Errorf("cpu shares cannot be negative")
	}
	if resources.CPUShares > 262144 {
		return fmt.Errorf("cpu shares cannot exceed 262144")
	}
	if resources.CPUQuota < 0 || math.IsNaN(resources.CPUQuota) || math.IsInf(resources.CPUQuota, 0) {
		return fmt.Errorf("cpu quota must be finite and non-negative")
	}
	if resources.MemoryMB < 0 {
		return fmt.Errorf("memory limit cannot be negative")
	}
	if resources.PidsLimit < 0 {
		return fmt.Errorf("pids limit cannot be negative")
	}
	return nil
}

func V2Weight(shares int64) int64 {
	if shares <= 0 {
		return 100
	}
	weight := 1 + ((shares-2)*9999)/262142
	if weight < 1 {
		return 1
	}
	if weight > 10000 {
		return 10000
	}
	return weight
}

func CPUQuota(quota float64) (string, string) {
	const period int64 = 100000
	if quota <= 0 {
		return "-1", strconv.FormatInt(period, 10)
	}
	value := int64(math.Round(quota * float64(period)))
	return strconv.FormatInt(value, 10), strconv.FormatInt(period, 10)
}

func CPUQuotaV2(quota float64) string {
	q, period := CPUQuota(quota)
	if q == "-1" {
		return "max " + period
	}
	return q + " " + period
}
