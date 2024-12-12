package insights

import (
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// systemResources defines the current system resources
type systemResources struct {
	MemoryPercentage float64
	CPUPercentage    float64
}

// collectSystemResources collects the current system resources
func collectSystemResources() systemResources {
	ret := systemResources{}

	// collect CPU percentage
	f, err := cpu.Percent(0, false)
	if err == nil && len(f) > 0 {
		ret.CPUPercentage = float64(uint64(f[0]*100)) / 10000
	}

	// collect memory percentage
	if memStats, err := mem.VirtualMemory(); err == nil {
		ret.MemoryPercentage = float64(uint64(memStats.UsedPercent)) / 100
	}

	return ret
}
