package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"epyc-pve/internal/affinity"
	"epyc-pve/internal/pve"
	"epyc-pve/internal/topology"
)

type step int

const (
	stepCoreType step = iota
	stepCoreCount
	stepStrategy
	stepManualCCD
	stepAction
	stepSelectVM
	stepConfirm
	stepApplying
	stepDone
	stepError
)

type Model struct {
	topo          *topology.CPUTopology
	step          step
	usePhysical   bool
	coresNeeded   int
	options       []affinity.Option
	selectedOpt   int
	selectedCCDs  []bool
	minCCDsNeeded int
	vms           []pve.VM
	selectedVM    int
	textInput     textinput.Model
	affinityStr   string
	err           error
	width         int
	height        int
}

func NewModel(topo *topology.CPUTopology) Model {
	ti := textinput.New()
	ti.Placeholder = "Enter number..."
	ti.Focus()
	ti.CharLimit = 10
	ti.Width = 20
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5"))
	ti.PromptStyle = lipgloss.NewStyle().Foreground(secondaryColor)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(primaryColor)

	return Model{
		topo:         topo,
		step:         stepCoreType,
		textInput:    ti,
		selectedCCDs: make([]bool, len(topo.CCDs)),
		width:        80,
		height:       24,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case applyResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepError
		} else {
			m.step = stepDone
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			m = m.moveCursor(-1)

		case "down", "j":
			m = m.moveCursor(1)

		case " ":
			if m.step == stepManualCCD && m.selectedOpt < len(m.selectedCCDs) {
				m.selectedCCDs[m.selectedOpt] = !m.selectedCCDs[m.selectedOpt]
			}

		case "enter":
			return m.handleEnter()

		case "esc":
			if m.step > stepCoreType && m.step < stepDone {
				m.step--
				if m.step == stepCoreCount {
					m.textInput.Focus()
					return m, textinput.Blink
				}
				if m.step == stepStrategy {
					m.selectedOpt = len(m.options) - 1
				}
				return m, nil
			}
		}
	}

	if m.step == stepCoreCount {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) moveCursor(delta int) Model {
	switch m.step {
	case stepCoreType:
		m.usePhysical = delta < 0
	case stepStrategy:
		m.selectedOpt += delta
		if m.selectedOpt < 0 {
			m.selectedOpt = len(m.options) - 1
		}
		if m.selectedOpt >= len(m.options) {
			m.selectedOpt = 0
		}
	case stepManualCCD:
		m.selectedOpt += delta
		if m.selectedOpt < 0 {
			m.selectedOpt = len(m.topo.CCDs) - 1
		}
		if m.selectedOpt >= len(m.topo.CCDs) {
			m.selectedOpt = 0
		}
	case stepAction:
		m.selectedOpt = (m.selectedOpt + 1) % 2
	case stepSelectVM:
		m.selectedVM += delta
		if m.selectedVM < 0 {
			m.selectedVM = len(m.vms) - 1
		}
		if m.selectedVM >= len(m.vms) {
			m.selectedVM = 0
		}
	case stepConfirm:
		m.selectedOpt = (m.selectedOpt + 1) % 2
	}
	return m
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepCoreType:
		m.step = stepCoreCount
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case stepCoreCount:
		val, err := strconv.Atoi(m.textInput.Value())
		maxCores := m.topo.TotalCores
		if !m.usePhysical {
			maxCores = m.topo.TotalCPUs
		}
		if err != nil || val < 1 || val > maxCores {
			return m, nil
		}
		m.coresNeeded = val

		req := &affinity.Request{
			CoresNeeded: m.coresNeeded,
			IncludeSMT:  !m.usePhysical,
			Topology:    m.topo,
		}
		options, err := affinity.Generate(req)
		if err != nil {
			m.err = err
			m.step = stepError
			return m, nil
		}
		m.options = options
		m.selectedOpt = 0

		physicalCoresNeeded := m.coresNeeded
		if !m.usePhysical && m.topo.HasSMT {
			physicalCoresNeeded = (m.coresNeeded + 1) / 2
		}
		m.minCCDsNeeded = affinity.MinCCDsNeeded(m.topo, physicalCoresNeeded)

		m.step = stepStrategy
		return m, nil

	case stepStrategy:
		selected := m.options[m.selectedOpt]

		if selected.Strategy == affinity.StrategyManual {
			m.selectedCCDs = make([]bool, len(m.topo.CCDs))
			m.selectedOpt = 0
			m.step = stepManualCCD
			return m, nil
		}

		if len(selected.CPUs) == 0 {
			return m, nil
		}
		m.affinityStr = selected.AffinityStr
		m.selectedOpt = 0
		m.step = stepAction
		return m, nil

	case stepManualCCD:
		selectedIndices := make([]int, 0)
		for i, selected := range m.selectedCCDs {
			if selected {
				selectedIndices = append(selectedIndices, i)
			}
		}

		if len(selectedIndices) < m.minCCDsNeeded {
			return m, nil
		}

		req := &affinity.Request{
			CoresNeeded: m.coresNeeded,
			IncludeSMT:  !m.usePhysical,
			Topology:    m.topo,
		}
		opt, err := affinity.GenerateManual(req, selectedIndices)
		if err != nil {
			m.err = err
			m.step = stepError
			return m, nil
		}
		m.affinityStr = opt.AffinityStr
		m.selectedOpt = 0
		m.step = stepAction
		return m, nil

	case stepAction:
		if m.selectedOpt == 0 {
			m.step = stepDone
			return m, nil
		}
		vms, err := pve.ListVMs()
		if err != nil {
			m.err = err
			m.step = stepError
			return m, nil
		}
		m.vms = vms
		m.selectedVM = 0
		m.step = stepSelectVM
		return m, nil

	case stepSelectVM:
		if len(m.vms) == 0 {
			return m, nil
		}
		m.selectedOpt = 0
		m.step = stepConfirm
		return m, nil

	case stepConfirm:
		if m.selectedOpt == 1 {
			return m, tea.Quit
		}
		m.step = stepApplying
		return m, m.applyAffinity()

	case stepDone, stepError:
		return m, tea.Quit
	}
	return m, nil
}

type applyResultMsg struct {
	err error
}

func (m Model) applyAffinity() tea.Cmd {
	return func() tea.Msg {
		vmid := m.vms[m.selectedVM].VMID
		err := pve.SetAffinity(vmid, m.affinityStr, false)
		return applyResultMsg{err: err}
	}
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.renderTopology())
	b.WriteString("\n\n")

	switch m.step {
	case stepCoreType:
		b.WriteString(m.renderCoreTypeSelection())
	case stepCoreCount:
		b.WriteString(m.renderCoreCountInput())
	case stepStrategy:
		b.WriteString(m.renderStrategySelection())
	case stepManualCCD:
		b.WriteString(m.renderManualCCDSelection())
	case stepAction:
		b.WriteString(m.renderActionSelection())
	case stepSelectVM:
		b.WriteString(m.renderVMSelection())
	case stepConfirm:
		b.WriteString(m.renderConfirmation())
	case stepApplying:
		b.WriteString(m.renderApplying())
	case stepDone:
		b.WriteString(m.renderSuccess())
	case stepError:
		b.WriteString(m.renderError())
	}

	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderHelp() string {
	keyStyle := lipgloss.NewStyle().Foreground(secondaryColor)
	sepStyle := dimStyle

	var parts []string
	parts = append(parts, keyStyle.Render("↑/↓")+sepStyle.Render(" navigate"))

	if m.step == stepManualCCD {
		parts = append(parts, keyStyle.Render("space")+sepStyle.Render(" toggle"))
		parts = append(parts, keyStyle.Render("enter")+sepStyle.Render(" confirm"))
	} else {
		parts = append(parts, keyStyle.Render("enter")+sepStyle.Render(" select"))
	}

	parts = append(parts, keyStyle.Render("esc")+sepStyle.Render(" back"))
	parts = append(parts, keyStyle.Render("q")+sepStyle.Render(" quit"))

	return strings.Join(parts, dimStyle.Render(" • "))
}

func (m Model) renderTopology() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" AMD EPYC/Ryzen CCD Affinity Tool "))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  %s %d    %s %d    %s %s    %s %s\n",
		packageStyle.Render("Cores:"), m.topo.TotalCores,
		vcpuStyle.Render("vCPUs:"), m.topo.TotalCPUs,
		dimStyle.Render("SMT:"), formatBool(m.topo.HasSMT),
		dimStyle.Render("Method:"), highlightStyle.Render(m.topo.DetectMethod)))
	b.WriteString("\n")

	for _, pkg := range m.topo.Packages {
		pkgCores := 0
		pkgThreads := 0
		for _, ccd := range pkg.CCDs {
			pkgCores += len(ccd.PhysicalCPUs)
			pkgThreads += len(ccd.AllCPUs)
		}

		b.WriteString(fmt.Sprintf("  %s %d  %s\n",
			packageStyle.Render("Package"), pkg.ID,
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
			b.WriteString(fmt.Sprintf("     %s %s %d%s  ", prefix, ccdStyle.Render("CCD"), ccd.ID, l3Info))
			b.WriteString(coreStyle.Render(affinity.FormatCPUs(ccd.PhysicalCPUs)))
			b.WriteString(dimStyle.Render(" / "))
			b.WriteString(vcpuStyle.Render(affinity.FormatCPUs(ccd.AllCPUs)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) renderCoreTypeSelection() string {
	var b strings.Builder
	b.WriteString(subtitleStyle.Render("? What type of CPU allocation?"))
	b.WriteString("\n\n")

	physicalLabel := fmt.Sprintf("Physical Cores (%d available)", m.topo.TotalCores)
	physicalDesc := "One vCPU per physical core"
	vcpuLabel := fmt.Sprintf("vCPUs/Threads (%d available)", m.topo.TotalCPUs)
	vcpuDesc := "Include SMT siblings"

	if m.usePhysical {
		b.WriteString(cursorStyle.Render("  ▸ "))
		b.WriteString(selectedStyle.Render(physicalLabel))
		b.WriteString("\n")
		b.WriteString("      " + dimStyle.Render(physicalDesc))
		b.WriteString("\n\n")
		b.WriteString("    ")
		b.WriteString(vcpuLabel)
		b.WriteString("\n")
		b.WriteString("      " + dimStyle.Render(vcpuDesc))
	} else {
		b.WriteString("    ")
		b.WriteString(physicalLabel)
		b.WriteString("\n")
		b.WriteString("      " + dimStyle.Render(physicalDesc))
		b.WriteString("\n\n")
		b.WriteString(cursorStyle.Render("  ▸ "))
		b.WriteString(selectedStyle.Render(vcpuLabel))
		b.WriteString("\n")
		b.WriteString("      " + dimStyle.Render(vcpuDesc))
	}

	return b.String()
}

func (m Model) renderCoreCountInput() string {
	var b strings.Builder

	coreType := "vCPUs"
	maxCores := m.topo.TotalCPUs
	if m.usePhysical {
		coreType = "physical cores"
		maxCores = m.topo.TotalCores
	}

	b.WriteString(subtitleStyle.Render(fmt.Sprintf("? How many %s?", coreType)))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Range: %s\n\n", highlightStyle.Render(fmt.Sprintf("1 - %d", maxCores))))
	b.WriteString("  > ")
	b.WriteString(m.textInput.View())

	return b.String()
}

func (m Model) renderStrategySelection() string {
	var b strings.Builder

	coreType := "vCPUs"
	if m.usePhysical {
		coreType = "cores"
	}
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("? Select strategy for %d %s", m.coresNeeded, coreType)))
	b.WriteString("\n\n")

	for i, opt := range m.options {
		available := len(opt.CPUs) > 0 || opt.Strategy == affinity.StrategyManual

		if i == m.selectedOpt {
			b.WriteString(cursorStyle.Render("  ▸ "))
			if available {
				b.WriteString(selectedStyle.Render(opt.Name))
			} else {
				b.WriteString(dimStyle.Render(opt.Name + " (unavailable)"))
			}
		} else {
			b.WriteString("    ")
			if available {
				b.WriteString(opt.Name)
			} else {
				b.WriteString(dimStyle.Render(opt.Name + " (unavailable)"))
			}
		}
		b.WriteString("\n")
		b.WriteString("      " + dimStyle.Render(opt.Description))
		b.WriteString("\n")

		if available && opt.Strategy != affinity.StrategyManual {
			b.WriteString(fmt.Sprintf("      CPUs: %s  CCDs: %d",
				vcpuStyle.Render(opt.AffinityStr),
				opt.CCDsUsed))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderManualCCDSelection() string {
	var b strings.Builder

	selectedCount := 0
	for _, selected := range m.selectedCCDs {
		if selected {
			selectedCount++
		}
	}

	b.WriteString(subtitleStyle.Render(fmt.Sprintf("? Select %d CCDs", m.minCCDsNeeded)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Selected: %d / %d required", selectedCount, m.minCCDsNeeded)))
	b.WriteString("\n\n")

	for i, ccd := range m.topo.CCDs {
		checkbox := "[ ]"
		if m.selectedCCDs[i] {
			checkbox = coreStyle.Render("[✓]")
		}

		if i == m.selectedOpt {
			b.WriteString(cursorStyle.Render("  ▸ "))
		} else {
			b.WriteString("    ")
		}

		b.WriteString(fmt.Sprintf("%s CCD %d  %s / %s",
			checkbox,
			ccd.ID,
			coreStyle.Render(affinity.FormatCPUs(ccd.PhysicalCPUs)),
			vcpuStyle.Render(affinity.FormatCPUs(ccd.AllCPUs))))
		b.WriteString("\n")
	}

	if selectedCount < m.minCCDsNeeded {
		b.WriteString("\n")
		b.WriteString(highlightStyle.Render(fmt.Sprintf("  Need %d more CCD(s)", m.minCCDsNeeded-selectedCount)))
	}

	return b.String()
}

func (m Model) renderActionSelection() string {
	var b strings.Builder

	b.WriteString(coreStyle.Render("✓ Affinity generated"))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(vcpuStyle.Render(m.affinityStr))
	b.WriteString("\n\n")

	b.WriteString(subtitleStyle.Render("? What next?"))
	b.WriteString("\n\n")

	if m.selectedOpt == 0 {
		b.WriteString(cursorStyle.Render("  ▸ "))
		b.WriteString(selectedStyle.Render("Copy and exit"))
		b.WriteString("\n")
		b.WriteString("    Apply to a VM")
	} else {
		b.WriteString("    Copy and exit")
		b.WriteString("\n")
		b.WriteString(cursorStyle.Render("  ▸ "))
		b.WriteString(selectedStyle.Render("Apply to a VM"))
	}

	return b.String()
}

func (m Model) renderVMSelection() string {
	var b strings.Builder
	b.WriteString(subtitleStyle.Render("? Select VM"))
	b.WriteString("\n\n")

	if len(m.vms) == 0 {
		b.WriteString(dimStyle.Render("  No VMs found"))
		return b.String()
	}

	for i, vm := range m.vms {
		var statusStyled string
		switch vm.Status {
		case "running":
			statusStyled = coreStyle.Render("●")
		case "stopped":
			statusStyled = lipgloss.NewStyle().Foreground(errorColor).Render("○")
		default:
			statusStyled = highlightStyle.Render("◐")
		}

		if i == m.selectedVM {
			b.WriteString(cursorStyle.Render("  ▸ "))
			b.WriteString(selectedStyle.Render(fmt.Sprintf("%d", vm.VMID)))
		} else {
			b.WriteString("    ")
			b.WriteString(fmt.Sprintf("%d", vm.VMID))
		}
		b.WriteString(fmt.Sprintf("  %-20s %s %s", vm.Name, statusStyled, vm.Status))
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderConfirmation() string {
	var b strings.Builder

	vm := m.vms[m.selectedVM]

	b.WriteString(subtitleStyle.Render("? Confirm"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  VM:       %s (%d)\n", highlightStyle.Render(vm.Name), vm.VMID))
	b.WriteString(fmt.Sprintf("  Affinity: %s\n", vcpuStyle.Render(m.affinityStr)))
	b.WriteString(fmt.Sprintf("  Command:  %s\n", dimStyle.Render(fmt.Sprintf("qm set %d --affinity %s", vm.VMID, m.affinityStr))))
	b.WriteString("\n")

	if m.selectedOpt == 0 {
		b.WriteString(cursorStyle.Render("  ▸ "))
		b.WriteString(selectedStyle.Render("Yes, apply"))
		b.WriteString("\n")
		b.WriteString("    No, cancel")
	} else {
		b.WriteString("    Yes, apply")
		b.WriteString("\n")
		b.WriteString(cursorStyle.Render("  ▸ "))
		b.WriteString(selectedStyle.Render("No, cancel"))
	}

	return b.String()
}

func (m Model) renderApplying() string {
	return "  Applying affinity configuration..."
}

func (m Model) renderSuccess() string {
	var b strings.Builder

	if len(m.vms) > 0 && m.selectedVM < len(m.vms) {
		vm := m.vms[m.selectedVM]
		b.WriteString(coreStyle.Render("✓ Applied"))
		b.WriteString(fmt.Sprintf(" to VM %d (%s)\n\n", vm.VMID, vm.Name))
		b.WriteString("  Affinity: ")
		b.WriteString(vcpuStyle.Render(m.affinityStr))
	} else {
		b.WriteString(coreStyle.Render("✓ Affinity configuration"))
		b.WriteString("\n\n")
		b.WriteString("  ")
		b.WriteString(vcpuStyle.Render(m.affinityStr))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  Use: qm set <vmid> --affinity %s", m.affinityStr)))
	}

	return b.String()
}

func (m Model) renderError() string {
	return lipgloss.NewStyle().Foreground(errorColor).Render(fmt.Sprintf("✗ Error: %v", m.err))
}

func formatBool(b bool) string {
	if b {
		return coreStyle.Render("Yes")
	}
	return lipgloss.NewStyle().Foreground(errorColor).Render("No")
}

func Run(topo *topology.CPUTopology) error {
	model := NewModel(topo)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	if m, ok := finalModel.(Model); ok && m.err != nil {
		return m.err
	}
	return nil
}
