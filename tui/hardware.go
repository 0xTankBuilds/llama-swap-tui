package tui

import (
	"fmt"
	"sort"
	"strings"

	"llama-swap-tui/api"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

func (m *Model) renderHardwareView() string {
	if m.errMsg != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatusError)).
			Render("Error: " + m.errMsg)
	}
	// Render header + body for backward compatibility
	header := m.renderHardwareHeader()
	body := m.renderHardwareBody()
	return header + body
}

func (m *Model) renderHardwareHeader() string {
	return m.renderSystemInfo()
}

func (m *Model) renderHardwareBody() string {
	var b strings.Builder

	// CPU section
	b.WriteString(m.renderCPUSection())

	// Memory section
	b.WriteString(m.renderMemorySection())

	// Accelerators / GPUs
	b.WriteString(m.renderGPUSection())

	// Live performance (SSE perfsys / perfgpu)
	b.WriteString(m.renderLivePerf())

	return b.String()
}

func (m *Model) renderSystemInfo() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  System Information") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", 40)) + "\n")

	arch := m.hardware.Architecture.Name
	if m.hardware.Architecture.RawName != nil {
		arch = *m.hardware.Architecture.RawName
	}

	osInfo := m.hardware.OperatingSystem.Family
	if m.hardware.OperatingSystem.Name != nil {
		osInfo = *m.hardware.OperatingSystem.Name
	}

	kind := m.hardware.Environment.Kind
	if m.hardware.Environment.Name != nil {
		kind = *m.hardware.Environment.Name
	}

	b.WriteString(fmt.Sprintf("  Arch: %-20s OS: %s\n", arch, osInfo))
	b.WriteString(fmt.Sprintf("  Env: %-24s\n", kind))
	if m.hardware.Capture.Method != "" {
		b.WriteString(fmt.Sprintf("  Detected via: %s\n", m.hardware.Capture.Method))
	}

	return b.String()
}

func (m *Model) renderCPUSection() string {
	var b strings.Builder
	barW := m.vp.Width - 16
	if barW < 10 {
		barW = 10
	}
	if barW > 40 {
		barW = 40
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  CPU") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", barW+12)) + "\n")

	if m.hardware.CPU.Model == nil {
		b.WriteString("  CPU info not available.\n")
		return b.String()
	}

	model := *m.hardware.CPU.Model
	vendor := ""
	if m.hardware.CPU.Vendor != nil {
		vendor = *m.hardware.CPU.Vendor + " "
	}
	cores := 0
	if m.hardware.CPU.PhysicalCoreCount != nil {
		cores = *m.hardware.CPU.PhysicalCoreCount
	}
	threads := 0
	if m.hardware.CPU.LogicalThreadCount != nil {
		threads = *m.hardware.CPU.LogicalThreadCount
	}
	sockets := 1
	if m.hardware.CPU.SocketCount != nil {
		sockets = *m.hardware.CPU.SocketCount
	}

	b.WriteString(fmt.Sprintf("  Model: %s%s\n", vendor, truncate(model, 30)))
	b.WriteString(fmt.Sprintf("  Cores/Threads: %d/%d (%d socket%s)\n", cores, threads, sockets, plural(sockets)))

	// Per-core CPU utilization from SSE
	if m.sysStat != nil && len(m.sysStat.CPUUtilPerCore) > 0 {
		b.WriteString("\n  Core Utilization:\n")
		maxBars := 40
		if len(m.sysStat.CPUUtilPerCore) < maxBars {
			maxBars = len(m.sysStat.CPUUtilPerCore)
		}
		for i := 0; i < maxBars; i++ {
			pct := m.sysStat.CPUUtilPerCore[i]
			bars := int(pct / 100.0 * float64(barW))
			color := colorStatusReady
			if pct > 80 {
				color = colorStatusError
			} else if pct > 50 {
				color = colorStatusStarting
			}
			b.WriteString(fmt.Sprintf("    Core %-4d [", i) +
				strings.Repeat("█", bars) +
				strings.Repeat(" ", barW-bars) +
				lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(fmt.Sprintf(" %5.1f%%", pct)) +
				"]\n")
		}
	}

	return b.String()
}

func (m *Model) renderMemorySection() string {
	var b strings.Builder
	barW := m.vp.Width - 16
	if barW < 10 {
		barW = 10
	}
	if barW > 40 {
		barW = 40
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  Memory") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", barW+12)) + "\n")

	totalMB := m.hardware.Memory.CapacityBytes >> 20

	// Live memory from SSE
	if m.sysStat != nil {
		usedPct := 0.0
		if m.sysStat.MemTotalMB > 0 {
			usedPct = float64(m.sysStat.MemUsedMB) / float64(m.sysStat.MemTotalMB) * 100
		}
		b.WriteString(fmt.Sprintf("  Used: %d MB / %d MB (%.1f%%)\n",
			m.sysStat.MemUsedMB, m.sysStat.MemTotalMB, usedPct))

		// Memory bar
		bars := int(usedPct / 100.0 * float64(barW))
		color := colorStatusReady
		if usedPct > 90 {
			color = colorStatusError
		} else if usedPct > 70 {
			color = colorStatusStarting
		}
		b.WriteString("    [" + strings.Repeat("█", bars) +
			strings.Repeat(" ", barW-bars) +
			lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(fmt.Sprintf(" %5.1f%%", usedPct)) +
			"]\n")

		if m.sysStat.SwapTotalMB > 0 {
			swapPct := float64(m.sysStat.SwapUsedMB) / float64(m.sysStat.SwapTotalMB) * 100
			b.WriteString(fmt.Sprintf("  Swap: %d MB / %d MB (%.1f%%)\n",
				m.sysStat.SwapUsedMB, m.sysStat.SwapTotalMB, swapPct))
			sbars := int(swapPct / 100.0 * float64(barW))
			scolor := colorStatusReady
			if swapPct > 80 {
				scolor = colorStatusError
			} else if swapPct > 50 {
				scolor = colorStatusStarting
			}
			b.WriteString("    [" + strings.Repeat("█", sbars) +
				strings.Repeat(" ", barW-sbars) +
				lipgloss.NewStyle().Foreground(lipgloss.Color(scolor)).Render(fmt.Sprintf(" %5.1f%%", swapPct)) +
				"]\n")
		}

		// Load average
		b.WriteString(fmt.Sprintf("  Load Avg: %.2f / %.2f / %.2f\n",
			m.sysStat.LoadAvg1, m.sysStat.LoadAvg5, m.sysStat.LoadAvg15))

		// Network I/O
		if len(m.sysStat.NetIO) > 0 {
			b.WriteString("\n  Network:\n")
			for _, net := range m.sysStat.NetIO {
				b.WriteString(fmt.Sprintf("    %-12s RX: %s  TX: %s\n",
					net.Name,
					humanBytes(net.BytesRecv),
					humanBytes(net.BytesSent),
				))
			}
		}
	} else {
		b.WriteString(fmt.Sprintf("  Total: %d MB (%.1f GB)\n", totalMB, float64(totalMB)/1024))
		b.WriteString("  (Live stats refresh every 5s)")
	}

	return b.String()
}

func (m *Model) renderGPUSection() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  Accelerators") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", 40)) + "\n")

	if len(m.hardware.Accelerators) == 0 {
		b.WriteString("  No accelerators detected.\n")
		return b.String()
	}

	for _, acc := range m.hardware.Accelerators {
		vendor := acc.Kind
		if acc.Vendor != nil {
			vendor = *acc.Vendor
		}
		model := acc.Kind
		if acc.Model != nil {
			model = *acc.Model
		}
		memStr := "-"
		if acc.Memory != nil && acc.Memory.CapacityBytes != nil {
			memStr = fmt.Sprintf("%d MB (%s)", *acc.Memory.CapacityBytes>>20, acc.Memory.Kind)
		}

		b.WriteString(fmt.Sprintf("\n  #%d %s %s\n", acc.Index, vendor, truncate(model, 30)))
		b.WriteString(fmt.Sprintf("    Memory: %s\n", memStr))
		if acc.Driver != nil && acc.Driver.Version != nil {
			b.WriteString(fmt.Sprintf("    Driver: %s\n", *acc.Driver.Version))
		}
		if acc.PowerLimitWatts != nil {
			b.WriteString(fmt.Sprintf("    Power Limit: %d W\n", *acc.PowerLimitWatts))
		}
	}

	return b.String()
}

func (m *Model) renderLivePerf() string {
	var b strings.Builder
	barW := m.vp.Width - 16
	if barW < 10 {
		barW = 10
	}
	if barW > 40 {
		barW = 40
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorAccent)).
		Render("  Live GPU Performance") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render("  " + strings.Repeat("-", barW+12)) + "\n")

	if len(m.gpuStats) == 0 {
		b.WriteString("  Waiting for GPU performance data...\n")
		return b.String()
	}

	ids := make([]int, 0, len(m.gpuStats))
	for id := range m.gpuStats {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		b.WriteString(m.renderGpuStatBlock(m.gpuStats[id], barW))
	}

	return b.String()
}

func (m *Model) renderGpuStatBlock(gs *api.GpuStat, barW int) string {
	var b strings.Builder

	memPct := 0.0
	if gs.MemTotalMB > 0 {
		memPct = float64(gs.MemUsedMB) / float64(gs.MemTotalMB) * 100
	}

	b.WriteString(fmt.Sprintf("  GPU %d: %s\n", gs.ID, gs.Name))
	b.WriteString(fmt.Sprintf("    Temp: %d°C (GPU) / %d°C (VRAM)\n", gs.TempC, gs.VramTempC))

	// GPU utilization bar
	gutilBars := int(gs.GpuUtilPct / 100.0 * float64(barW))
	gutilColor := colorStatusReady
	if gs.GpuUtilPct > 80 {
		gutilColor = colorStatusError
	} else if gs.GpuUtilPct > 50 {
		gutilColor = colorStatusStarting
	}
	b.WriteString(fmt.Sprintf("    GPU Util: [%s%s] %s\n",
		strings.Repeat("█", gutilBars),
		strings.Repeat(" ", barW-gutilBars),
		lipgloss.NewStyle().Foreground(lipgloss.Color(gutilColor)).Render(fmt.Sprintf("%.1f%%", gs.GpuUtilPct)),
	))

	// VRAM bar
	vramBars := int(memPct / 100.0 * float64(barW))
	vramColor := colorStatusReady
	if memPct > 90 {
		vramColor = colorStatusError
	} else if memPct > 70 {
		vramColor = colorStatusStarting
	}
	b.WriteString(fmt.Sprintf("    VRAM:   [%s%s] %s\n",
		strings.Repeat("█", vramBars),
		strings.Repeat(" ", barW-vramBars),
		lipgloss.NewStyle().Foreground(lipgloss.Color(vramColor)).Render(fmt.Sprintf("%d/%d MB (%.1f%%)", gs.MemUsedMB, gs.MemTotalMB, memPct)),
	))

	// Fan speed
	if gs.FanSpeedPct > 0 {
		fanBars := int(gs.FanSpeedPct / 100.0 * float64(barW))
		b.WriteString(fmt.Sprintf("    Fan:    [%s%s] %.1f%%\n",
			strings.Repeat("█", fanBars),
			strings.Repeat(" ", barW-fanBars),
			gs.FanSpeedPct,
		))
	}

	// Power draw
	if gs.PowerDrawW > 0 {
		b.WriteString(fmt.Sprintf("    Power:  %.1f W", gs.PowerDrawW))
		if limit, ok := m.gpuPowerLimit(gs.ID); ok {
			powerPct := gs.PowerDrawW / float64(limit) * 100
			b.WriteString(fmt.Sprintf(" (%.1f%% of limit)", powerPct))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// gpuPowerLimit looks up the power limit (W) for a GPU by its index in the
// hardware snapshot. Returns ok=false when no accelerator matches.
func (m *Model) gpuPowerLimit(id int) (int, bool) {
	for _, acc := range m.hardware.Accelerators {
		if acc.Index == id {
			if acc.PowerLimitWatts != nil {
				return *acc.PowerLimitWatts, true
			}
			return 0, false
		}
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func humanBytes(b int64) string {
	const (
		_ = 1 << (iota * 10)
		KB
		MB
		GB
		TB
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.1f TB", float64(b)/TB)
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/KB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
