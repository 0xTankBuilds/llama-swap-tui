package tui

import (
	"fmt"
	"strings"

	"llama-swap-tui/api"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Hardware view state
// ---------------------------------------------------------------------------

type hardwareView struct {
	sysStat     *api.SysStat
	gpuStat     *api.GpuStat
	hwSnap      api.HardwareSnapshot
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

func (m *Model) renderHardwareView() string {
	if m.errMsg != "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f44")).
			Render("Error: " + m.errMsg)
	}

	var b strings.Builder

	// System info header
	b.WriteString(m.renderSystemInfo())

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
		Foreground(lipgloss.Color("#0BB")).
		Render("  System Information") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444")).
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
		Foreground(lipgloss.Color("#0BB")).
		Render("  CPU") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444")).
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
			color := "#0F0"
			if pct > 80 {
				color = "#F44"
			} else if pct > 50 {
				color = "#FF0"
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
		Foreground(lipgloss.Color("#0BB")).
		Render("  Memory") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444")).
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
		color := "#0F0"
		if usedPct > 90 {
			color = "#F44"
		} else if usedPct > 70 {
			color = "#FF0"
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
			scolor := "#0F0"
			if swapPct > 80 {
				scolor = "#F44"
			} else if swapPct > 50 {
				scolor = "#FF0"
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
		b.WriteString("  (Live stats from SSE perfsys events)")
	}

	return b.String()
}

func (m *Model) renderGPUSection() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#0BB")).
		Render("  Accelerators") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444")).
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
		Foreground(lipgloss.Color("#0BB")).
		Render("  Live GPU Performance (SSE)") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444")).
		Render("  " + strings.Repeat("-", barW+12)) + "\n")

	if m.gpuStat == nil {
		b.WriteString("  Waiting for GPU performance data...\n")
		return b.String()
	}

	gs := *m.gpuStat
	memPct := 0.0
	if gs.MemTotalMB > 0 {
		memPct = float64(gs.MemUsedMB) / float64(gs.MemTotalMB) * 100
	}

	b.WriteString(fmt.Sprintf("  GPU: %s (ID: %d)\n", gs.Name, gs.ID))
	b.WriteString(fmt.Sprintf("  Temperature: %d°C (GPU) / %d°C (VRAM)\n", gs.TempC, gs.VramTempC))

	// GPU utilization bar
	gutilBars := int(gs.GpuUtilPct / 100.0 * float64(barW))
	gutilColor := "#0F0"
	if gs.GpuUtilPct > 80 {
		gutilColor = "#F44"
	} else if gs.GpuUtilPct > 50 {
		gutilColor = "#FF0"
	}
	b.WriteString(fmt.Sprintf("  GPU Util: [%s%s] %s\n",
		strings.Repeat("█", gutilBars),
		strings.Repeat(" ", barW-gutilBars),
		lipgloss.NewStyle().Foreground(lipgloss.Color(gutilColor)).Render(fmt.Sprintf("%.1f%%", gs.GpuUtilPct)),
	))

	// VRAM bar
	vramBars := int(memPct / 100.0 * float64(barW))
	vramColor := "#0F0"
	if memPct > 90 {
		vramColor = "#F44"
	} else if memPct > 70 {
		vramColor = "#FF0"
	}
	b.WriteString(fmt.Sprintf("  VRAM:   [%s%s] %s\n",
		strings.Repeat("█", vramBars),
		strings.Repeat(" ", barW-vramBars),
		lipgloss.NewStyle().Foreground(lipgloss.Color(vramColor)).Render(fmt.Sprintf("%d/%d MB (%.1f%%)", gs.MemUsedMB, gs.MemTotalMB, memPct)),
	))

	// Fan speed
	if gs.FanSpeedPct > 0 {
		fanBars := int(gs.FanSpeedPct / 100.0 * float64(barW))
		b.WriteString(fmt.Sprintf("  Fan:    [%s%s] %.1f%%\n",
			strings.Repeat("█", fanBars),
			strings.Repeat(" ", barW-fanBars),
			gs.FanSpeedPct,
		))
	}

	// Power draw
	if gs.PowerDrawW > 0 {
		powerPct := 0.0
		if m.hardware.Accelerators[gs.ID].PowerLimitWatts != nil {
			powerPct = gs.PowerDrawW / float64(*m.hardware.Accelerators[gs.ID].PowerLimitWatts) * 100
		}
		b.WriteString(fmt.Sprintf("  Power:  %.1f W", gs.PowerDrawW))
		if powerPct > 0 {
			b.WriteString(fmt.Sprintf(" (%.1f%% of limit)", powerPct))
		}
		b.WriteString("\n")
	}

	return b.String()
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
