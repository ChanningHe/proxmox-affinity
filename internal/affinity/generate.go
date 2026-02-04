package affinity

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"epyc-pve/internal/topology"
)

func Generate(req *Request) ([]Option, error) {
	if req == nil || req.Topology == nil {
		return nil, errors.New("topology is required")
	}
	if req.CoresNeeded <= 0 {
		return nil, errors.New("cores needed must be greater than zero")
	}

	physicalCoresNeeded := req.CoresNeeded
	if req.IncludeSMT && req.Topology.HasSMT {
		physicalCoresNeeded = (req.CoresNeeded + 1) / 2
	}

	if physicalCoresNeeded > req.Topology.TotalCores {
		return nil, fmt.Errorf("not enough cores. need %d physical cores for %d vCPUs, but only %d available",
			physicalCoresNeeded, req.CoresNeeded, req.Topology.TotalCores)
	}

	switch req.Topology.Architecture {
	case topology.ArchIntelHybrid:
		return generateIntelOptions(req, physicalCoresNeeded)
	default:
		return generateAMDOptions(req, physicalCoresNeeded)
	}
}

func generateAMDOptions(req *Request, physicalCoresNeeded int) ([]Option, error) {
	options := []Option{
		*generateSingleCCD(req, physicalCoresNeeded),
		*generateDistributed(req, physicalCoresNeeded),
		*generateSequential(req, physicalCoresNeeded),
		*generateRandom(req, physicalCoresNeeded),
		*generateManualPlaceholder(req, physicalCoresNeeded),
	}

	for i := range options {
		options[i].AffinityStr = FormatCPUs(options[i].CPUs)
	}

	return options, nil
}

func generateIntelOptions(req *Request, physicalCoresNeeded int) ([]Option, error) {
	options := []Option{
		*generatePCoresOnly(req, physicalCoresNeeded),
		*generateECoresOnly(req, physicalCoresNeeded),
		*generateAllCores(req, physicalCoresNeeded),
		*generateSequential(req, physicalCoresNeeded),
		*generateManualPlaceholder(req, physicalCoresNeeded),
	}

	for i := range options {
		options[i].AffinityStr = FormatCPUs(options[i].CPUs)
	}

	return options, nil
}

func generatePCoresOnly(req *Request, physicalCoresNeeded int) *Option {
	option := &Option{
		Strategy:    StrategyPCoresOnly,
		Name:        "P-Cores Only",
		Description: "Use only Performance cores (best single-thread)",
	}

	pCores := req.Topology.GetPCoresCPUs()
	if len(pCores) < physicalCoresNeeded {
		option.Description = fmt.Sprintf("Unavailable: only %d P-cores, need %d", len(pCores), physicalCoresNeeded)
		return option
	}

	selectedPhysical := pCores[:physicalCoresNeeded]
	option.CPUs = expandToVCPUs(selectedPhysical, req.IncludeSMT, req.Topology)
	option.CCDsUsed = 1
	return option
}

func generateECoresOnly(req *Request, physicalCoresNeeded int) *Option {
	option := &Option{
		Strategy:    StrategyECoresOnly,
		Name:        "E-Cores Only",
		Description: "Use only Efficiency cores (power efficient)",
	}

	eCores := req.Topology.GetECoresCPUs()
	if len(eCores) < physicalCoresNeeded {
		option.Description = fmt.Sprintf("Unavailable: only %d E-cores, need %d", len(eCores), physicalCoresNeeded)
		return option
	}

	selectedPhysical := eCores[:physicalCoresNeeded]
	option.CPUs = expandToVCPUs(selectedPhysical, req.IncludeSMT, req.Topology)
	option.CCDsUsed = 1
	return option
}

func generateAllCores(req *Request, physicalCoresNeeded int) *Option {
	option := &Option{
		Strategy:    StrategyAllCores,
		Name:        "All Cores",
		Description: "Use both P-cores and E-cores (maximum throughput)",
	}

	pCores := req.Topology.GetPCoresCPUs()
	eCores := req.Topology.GetECoresCPUs()

	selectedPhysical := make([]int, 0, physicalCoresNeeded)
	selectedPhysical = append(selectedPhysical, pCores...)
	selectedPhysical = append(selectedPhysical, eCores...)
	sort.Ints(selectedPhysical)

	if len(selectedPhysical) > physicalCoresNeeded {
		selectedPhysical = selectedPhysical[:physicalCoresNeeded]
	}

	option.CPUs = expandToVCPUs(selectedPhysical, req.IncludeSMT, req.Topology)
	option.CCDsUsed = countCCDsUsedByPhysical(selectedPhysical, req.Topology)
	return option
}

func generateSingleCCD(req *Request, physicalCoresNeeded int) *Option {
	option := &Option{
		Strategy:    StrategySingleCCD,
		Name:        "Single CCD",
		Description: "All cores from one CCD (best cache locality)",
	}

	for _, cg := range req.Topology.CoreGroups {
		if len(cg.PhysicalCPUs) >= physicalCoresNeeded {
			physicalCores := make([]int, physicalCoresNeeded)
			copy(physicalCores, cg.PhysicalCPUs[:physicalCoresNeeded])

			option.CPUs = expandToVCPUs(physicalCores, req.IncludeSMT, req.Topology)
			option.CCDsUsed = 1
			return option
		}
	}

	option.Description = fmt.Sprintf("Unavailable: no single CCD has %d cores", physicalCoresNeeded)
	return option
}

func generateDistributed(req *Request, physicalCoresNeeded int) *Option {
	option := &Option{
		Strategy:    StrategyDistributed,
		Name:        "Distributed",
		Description: "Spread cores across CCDs",
	}

	coreGroups := sortedCoreGroups(req.Topology.CoreGroups)
	selectedPhysical := make([]int, 0, physicalCoresNeeded)
	usedCCDs := make(map[int]struct{})
	positions := make([]int, len(coreGroups))

	for len(selectedPhysical) < physicalCoresNeeded {
		progress := false
		for i, cg := range coreGroups {
			if len(selectedPhysical) >= physicalCoresNeeded {
				break
			}
			if positions[i] >= len(cg.PhysicalCPUs) {
				continue
			}
			selectedPhysical = append(selectedPhysical, cg.PhysicalCPUs[positions[i]])
			positions[i]++
			usedCCDs[i] = struct{}{}
			progress = true
		}
		if !progress {
			break
		}
	}

	option.CPUs = expandToVCPUs(selectedPhysical, req.IncludeSMT, req.Topology)
	option.CCDsUsed = len(usedCCDs)
	return option
}

func generateSequential(req *Request, physicalCoresNeeded int) *Option {
	option := &Option{
		Strategy:    StrategySequential,
		Name:        "Sequential",
		Description: "First N cores from consecutive CCDs",
	}

	allPhysical := allPhysicalCPUsSorted(req.Topology)
	selectedPhysical := allPhysical
	if len(selectedPhysical) > physicalCoresNeeded {
		selectedPhysical = selectedPhysical[:physicalCoresNeeded]
	}

	option.CPUs = expandToVCPUs(selectedPhysical, req.IncludeSMT, req.Topology)
	option.CCDsUsed = countCCDsUsedByPhysical(selectedPhysical, req.Topology)
	return option
}

func generateRandom(req *Request, physicalCoresNeeded int) *Option {
	option := &Option{
		Strategy:    StrategyRandom,
		Name:        "Random",
		Description: "Randomly select from minimum CCDs needed",
	}

	coreGroups := req.Topology.CoreGroups
	if len(coreGroups) == 0 {
		return option
	}

	coresPerCCD := len(coreGroups[0].PhysicalCPUs)
	minCCDsNeeded := (physicalCoresNeeded + coresPerCCD - 1) / coresPerCCD

	if minCCDsNeeded > len(coreGroups) {
		minCCDsNeeded = len(coreGroups)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	ccdIndices := make([]int, len(coreGroups))
	for i := range ccdIndices {
		ccdIndices[i] = i
	}
	rng.Shuffle(len(ccdIndices), func(i, j int) {
		ccdIndices[i], ccdIndices[j] = ccdIndices[j], ccdIndices[i]
	})

	selectedCCDs := ccdIndices[:minCCDsNeeded]
	sort.Ints(selectedCCDs)

	selectedPhysical := make([]int, 0, physicalCoresNeeded)
	for _, ccdIdx := range selectedCCDs {
		cg := coreGroups[ccdIdx]
		for _, phys := range cg.PhysicalCPUs {
			if len(selectedPhysical) >= physicalCoresNeeded {
				break
			}
			selectedPhysical = append(selectedPhysical, phys)
		}
	}

	option.CPUs = expandToVCPUs(selectedPhysical, req.IncludeSMT, req.Topology)
	option.CCDsUsed = minCCDsNeeded
	return option
}

func generateManualPlaceholder(req *Request, physicalCoresNeeded int) *Option {
	coreGroups := req.Topology.CoreGroups
	coresPerCCD := 0
	if len(coreGroups) > 0 {
		coresPerCCD = len(coreGroups[0].PhysicalCPUs)
	}
	minCCDsNeeded := 1
	if coresPerCCD > 0 {
		minCCDsNeeded = (physicalCoresNeeded + coresPerCCD - 1) / coresPerCCD
	}

	return &Option{
		Strategy:    StrategyManual,
		Name:        "Manual",
		Description: fmt.Sprintf("Select %d CCDs manually", minCCDsNeeded),
		CCDsUsed:    minCCDsNeeded,
	}
}

func GenerateManual(req *Request, selectedCCDIndices []int) (*Option, error) {
	if req == nil || req.Topology == nil {
		return nil, errors.New("topology is required")
	}
	if len(selectedCCDIndices) == 0 {
		return nil, errors.New("no CCDs selected")
	}

	physicalCoresNeeded := req.CoresNeeded
	if req.IncludeSMT && req.Topology.HasSMT {
		physicalCoresNeeded = (req.CoresNeeded + 1) / 2
	}

	coreGroups := req.Topology.CoreGroups
	sort.Ints(selectedCCDIndices)

	selectedPhysical := make([]int, 0, physicalCoresNeeded)
	for _, ccdIdx := range selectedCCDIndices {
		if ccdIdx < 0 || ccdIdx >= len(coreGroups) {
			continue
		}
		cg := coreGroups[ccdIdx]
		for _, phys := range cg.PhysicalCPUs {
			if len(selectedPhysical) >= physicalCoresNeeded {
				break
			}
			selectedPhysical = append(selectedPhysical, phys)
		}
	}

	if len(selectedPhysical) < physicalCoresNeeded {
		return nil, fmt.Errorf("selected CCDs only have %d cores, need %d", len(selectedPhysical), physicalCoresNeeded)
	}

	option := &Option{
		Strategy:    StrategyManual,
		Name:        "Manual",
		Description: fmt.Sprintf("Manually selected %d CCDs", len(selectedCCDIndices)),
		CCDsUsed:    len(selectedCCDIndices),
	}
	option.CPUs = expandToVCPUs(selectedPhysical, req.IncludeSMT, req.Topology)
	option.AffinityStr = FormatCPUs(option.CPUs)
	return option, nil
}

func MinCCDsNeeded(topo *topology.CPUTopology, physicalCoresNeeded int) int {
	if len(topo.CoreGroups) == 0 {
		return 0
	}
	coresPerCCD := len(topo.CoreGroups[0].PhysicalCPUs)
	if coresPerCCD == 0 {
		return 0
	}
	return (physicalCoresNeeded + coresPerCCD - 1) / coresPerCCD
}

func expandToVCPUs(physicalCores []int, includeSMT bool, topo *topology.CPUTopology) []int {
	if !includeSMT || !topo.HasSMT {
		result := make([]int, len(physicalCores))
		copy(result, physicalCores)
		sort.Ints(result)
		return result
	}

	physicalToSiblings := make(map[int][]int)
	for _, cg := range topo.CoreGroups {
		numPhysical := len(cg.PhysicalCPUs)
		for i, phys := range cg.PhysicalCPUs {
			siblings := []int{phys}
			if i+numPhysical < len(cg.AllCPUs) {
				siblings = append(siblings, cg.AllCPUs[i+numPhysical])
			}
			physicalToSiblings[phys] = siblings
		}
	}

	result := make([]int, 0, len(physicalCores)*2)
	for _, phys := range physicalCores {
		if siblings, ok := physicalToSiblings[phys]; ok {
			result = append(result, siblings...)
		} else {
			result = append(result, phys)
		}
	}

	sort.Ints(result)
	return dedupeSorted(result)
}

func FormatCPUs(cpus []int) string {
	if len(cpus) == 0 {
		return ""
	}
	sorted := make([]int, len(cpus))
	copy(sorted, cpus)
	sort.Ints(sorted)
	sorted = dedupeSorted(sorted)

	parts := make([]string, 0, len(sorted))
	start := sorted[0]
	prev := sorted[0]
	for i := 1; i < len(sorted); i++ {
		current := sorted[i]
		if current == prev+1 {
			prev = current
			continue
		}
		parts = append(parts, formatRange(start, prev))
		start = current
		prev = current
	}
	parts = append(parts, formatRange(start, prev))

	return strings.Join(parts, ",")
}

func formatRange(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "-" + strconv.Itoa(end)
}

func allPhysicalCPUsSorted(topo *topology.CPUTopology) []int {
	var result []int
	for _, cg := range topo.CoreGroups {
		result = append(result, cg.PhysicalCPUs...)
	}
	sort.Ints(result)
	return dedupeSorted(result)
}

func sortedCoreGroups(coreGroups []topology.CoreGroup) []topology.CoreGroup {
	list := make([]topology.CoreGroup, len(coreGroups))
	copy(list, coreGroups)
	sort.Slice(list, func(i, j int) bool {
		if list[i].PackageID == list[j].PackageID {
			return list[i].ID < list[j].ID
		}
		return list[i].PackageID < list[j].PackageID
	})
	return list
}

func countCCDsUsedByPhysical(physicalCores []int, topo *topology.CPUTopology) int {
	physicalSet := make(map[int]struct{})
	for _, p := range physicalCores {
		physicalSet[p] = struct{}{}
	}

	usedCCDs := make(map[int]struct{})
	for i, cg := range topo.CoreGroups {
		for _, p := range cg.PhysicalCPUs {
			if _, ok := physicalSet[p]; ok {
				usedCCDs[i] = struct{}{}
				break
			}
		}
	}
	return len(usedCCDs)
}

func dedupeSorted(values []int) []int {
	if len(values) == 0 {
		return values
	}
	result := make([]int, 0, len(values))
	last := values[0] - 1
	for _, value := range values {
		if value == last {
			continue
		}
		result = append(result, value)
		last = value
	}
	return result
}
