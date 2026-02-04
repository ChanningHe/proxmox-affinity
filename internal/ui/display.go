package ui

import (
	"fmt"
	"os"
	"strings"

	"epyc-pve/internal/affinity"
	"epyc-pve/internal/pve"
	"epyc-pve/internal/topology"
)

func PrintTopology(topo *topology.CPUTopology) {
	if topo == nil {
		fmt.Println(errorBoxStyle.Render("CPU topology unavailable"))
		return
	}

	var b strings.Builder

	title := titleStyle.Render("🖥  AMD EPYC/Ryzen CPU Topology")
	b.WriteString(title)
	b.WriteString("\n\n")

	var info strings.Builder
	info.WriteString(fmt.Sprintf("  %s %d    %s %d    %s %s    %s %s\n",
		packageStyle.Render("Cores:"), topo.TotalCores,
		vcpuStyle.Render("vCPUs:"), topo.TotalCPUs,
		dimStyle.Render("SMT:"), formatBoolDisplay(topo.HasSMT),
		dimStyle.Render("Method:"), highlightStyle.Render(topo.DetectMethod)))
	info.WriteString("\n")

	for _, pkg := range topo.Packages {
		pkgCores := 0
		pkgThreads := 0
		for _, ccd := range pkg.CCDs {
			pkgCores += len(ccd.PhysicalCPUs)
			pkgThreads += len(ccd.AllCPUs)
		}
		info.WriteString(fmt.Sprintf("  %s %d  %s\n",
			packageStyle.Render("📦 Package"), pkg.ID,
			dimStyle.Render(fmt.Sprintf("(%d cores, %d threads)", pkgCores, pkgThreads))))

		for i, ccd := range pkg.CCDs {
			prefix := "├─"
			if i == len(pkg.CCDs)-1 {
				prefix = "└─"
			}
			l3Info := ""
			if ccd.L3CacheID >= 0 {
				l3Info = dimStyle.Render(fmt.Sprintf(" [L3#%d]", ccd.L3CacheID))
			}
			info.WriteString(fmt.Sprintf("     %s %s %d%s  ", prefix, ccdStyle.Render("CCD"), ccd.ID, l3Info))
			info.WriteString(coreStyle.Render(affinity.FormatCPUs(ccd.PhysicalCPUs)))
			info.WriteString(dimStyle.Render(" / "))
			info.WriteString(vcpuStyle.Render(affinity.FormatCPUs(ccd.AllCPUs)))
			info.WriteString("\n")
		}
	}

	fmt.Println(boxStyle.Render(b.String() + info.String()))
}

func PrintOptions(options []affinity.Option, usePhysical bool) {
	coreType := "vCPUs"
	if usePhysical {
		coreType = "Physical Cores"
	}

	fmt.Println(subtitleStyle.Render("Affinity Options"))
	fmt.Println()

	for i, option := range options {
		available := len(option.CPUs) > 0

		var status string
		if available {
			status = coreStyle.Render("✓")
		} else {
			status = dimStyle.Render("✗")
		}

		fmt.Printf("  %s [%d] %s\n", status, i+1, highlightStyle.Render(option.Name))
		fmt.Printf("      %s\n", dimStyle.Render(option.Description))

		if available {
			fmt.Printf("      %s: %s  CCDs: %d\n", coreType, vcpuStyle.Render(option.AffinityStr), option.CCDsUsed)
		} else {
			fmt.Printf("      %s: %s\n", coreType, dimStyle.Render("unavailable"))
		}
		fmt.Println()
	}
}

func PrintVMs(vms []pve.VM) {
	fmt.Println(subtitleStyle.Render("Available VMs"))
	fmt.Println()

	if len(vms) == 0 {
		fmt.Println(dimStyle.Render("  No VMs found"))
		return
	}

	for i, vm := range vms {
		status := vm.Status
		var statusStyled string
		switch status {
		case "running":
			statusStyled = coreStyle.Render(status)
		case "stopped":
			statusStyled = errorBoxStyle.Render(status)
		default:
			statusStyled = highlightStyle.Render(status)
		}

		fmt.Printf("  [%d] %-6d %-25s %s\n", i+1, vm.VMID, vm.Name, statusStyled)
	}
	fmt.Println()
}

func PrintSuccess(vmid int, affinityStr string) {
	content := fmt.Sprintf("✓ Successfully applied affinity to VM %d\n\n  Affinity: %s", vmid, affinityStr)
	fmt.Println()
	fmt.Println(successBoxStyle.Render(content))
	fmt.Println()
}

func PrintError(err error) {
	content := fmt.Sprintf("✗ Error: %v", err)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, errorBoxStyle.Render(content))
	fmt.Fprintln(os.Stderr)
}

func PrintDryRun(vmid int, affinityStr string) {
	content := fmt.Sprintf("DRY RUN - Would apply:\n\n  VM: %d\n  Affinity: %s\n  Command: qm set %d --affinity %s",
		vmid, affinityStr, vmid, affinityStr)
	fmt.Println()
	fmt.Println(boxStyle.Render(content))
	fmt.Println()
}

func formatBoolDisplay(b bool) string {
	if b {
		return coreStyle.Render("Yes")
	}
	return dimStyle.Render("No")
}
