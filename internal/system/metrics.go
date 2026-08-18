package system

import (
	"context"
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

type Metrics struct {
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryUsedGB  float64   `json:"memory_used_gb"`
	MemoryTotalGB float64   `json:"memory_total_gb"`
	MemoryPercent float64   `json:"memory_percent"`
	UptimeSeconds uint64    `json:"uptime_seconds"`
	GPU           GPU       `json:"gpu"`
	CollectedAt   time.Time `json:"collected_at"`
}

type GPU struct {
	Available     bool    `json:"available"`
	Name          string  `json:"name,omitempty"`
	Utilization   float64 `json:"utilization"`
	MemoryUsedGB  float64 `json:"memory_used_gb"`
	MemoryTotalGB float64 `json:"memory_total_gb"`
	Temperature   float64 `json:"temperature"`
	PowerWatts    float64 `json:"power_watts"`
}

func Collect(ctx context.Context) Metrics {
	result := Metrics{CollectedAt: time.Now().UTC()}
	if percentages, err := cpu.PercentWithContext(ctx, 150*time.Millisecond, false); err == nil && len(percentages) > 0 {
		result.CPUPercent = round(percentages[0], 1)
	}
	if memory, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		result.MemoryUsedGB = bytesToGB(memory.Used)
		result.MemoryTotalGB = bytesToGB(memory.Total)
		result.MemoryPercent = round(memory.UsedPercent, 1)
	}
	if uptime, err := host.UptimeWithContext(ctx); err == nil {
		result.UptimeSeconds = uptime
	}
	result.GPU = collectGPU(ctx)
	return result
}

func collectGPU(parent context.Context) GPU {
	ctx, cancel := context.WithTimeout(parent, 2500*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, "nvidia-smi.exe", "--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw", "--format=csv,noheader,nounits")
	configureCommand(command)
	output, err := command.Output()
	if err != nil {
		return GPU{}
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimSpace(string(output))))
	record, err := reader.Read()
	if err != nil || len(record) < 6 {
		return GPU{}
	}
	parse := func(value string) float64 {
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsed
	}
	return GPU{
		Available:     true,
		Name:          strings.TrimSpace(record[0]),
		Utilization:   parse(record[1]),
		MemoryUsedGB:  round(parse(record[2])/1024, 1),
		MemoryTotalGB: round(parse(record[3])/1024, 1),
		Temperature:   parse(record[4]),
		PowerWatts:    round(parse(record[5]), 1),
	}
}

func bytesToGB(value uint64) float64 { return round(float64(value)/(1024*1024*1024), 1) }
func round(value float64, places int) float64 {
	power := 1.0
	for range places {
		power *= 10
	}
	return float64(int(value*power+0.5)) / power
}
