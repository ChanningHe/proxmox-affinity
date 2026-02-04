package topology

type Architecture string

const (
	ArchAMD         Architecture = "amd"
	ArchIntelHybrid Architecture = "intel_hybrid"
	ArchGeneric     Architecture = "generic"
)

type CoreType string

const (
	CoreTypePerformance CoreType = "performance"
	CoreTypeEfficiency  CoreType = "efficiency"
	CoreTypeUnknown     CoreType = "unknown"
)

type CPUTopology struct {
	Architecture Architecture `json:"architecture"`
	TotalCPUs    int          `json:"total_cpus"`
	TotalCores   int          `json:"total_cores"`
	HasSMT       bool         `json:"has_smt"`
	Packages     []Package    `json:"packages"`
	CoreGroups   []CoreGroup  `json:"core_groups"`
	DetectMethod string       `json:"detect_method"`
}

type Package struct {
	ID         int         `json:"id"`
	CoreGroups []CoreGroup `json:"core_groups"`
}

type CoreGroup struct {
	ID           int      `json:"id"`
	PackageID    int      `json:"package_id"`
	Type         CoreType `json:"type"`
	Name         string   `json:"name"`
	L3CacheID    int      `json:"l3_cache_id"`
	PhysicalCPUs []int    `json:"physical_cpus"`
	AllCPUs      []int    `json:"all_cpus"`
}

func (g *CoreGroup) IsCCD() bool {
	return g.Type == CoreTypeUnknown && g.L3CacheID >= 0
}

func (g *CoreGroup) IsPCore() bool {
	return g.Type == CoreTypePerformance
}

func (g *CoreGroup) IsECore() bool {
	return g.Type == CoreTypeEfficiency
}

type CPUInfo struct {
	ID             int
	PackageID      int
	CoreID         int
	ClusterID      int
	DieID          int
	L3CacheID      int
	ThreadSiblings []int
	IsFirstThread  bool
	CoreType       CoreType
	Capacity       int
}

func (t *CPUTopology) CCDs() []CoreGroup {
	var ccds []CoreGroup
	for _, g := range t.CoreGroups {
		if g.IsCCD() {
			ccds = append(ccds, g)
		}
	}
	return ccds
}

func (t *CPUTopology) PCores() []CoreGroup {
	var pcores []CoreGroup
	for _, g := range t.CoreGroups {
		if g.IsPCore() {
			pcores = append(pcores, g)
		}
	}
	return pcores
}

func (t *CPUTopology) ECores() []CoreGroup {
	var ecores []CoreGroup
	for _, g := range t.CoreGroups {
		if g.IsECore() {
			ecores = append(ecores, g)
		}
	}
	return ecores
}

func (t *CPUTopology) GetPCoresCPUs() []int {
	var cpus []int
	for _, g := range t.CoreGroups {
		if g.IsPCore() {
			cpus = append(cpus, g.PhysicalCPUs...)
		}
	}
	return cpus
}

func (t *CPUTopology) GetECoresCPUs() []int {
	var cpus []int
	for _, g := range t.CoreGroups {
		if g.IsECore() {
			cpus = append(cpus, g.PhysicalCPUs...)
		}
	}
	return cpus
}

func (t *CPUTopology) GetAllPCoresVCPUs() []int {
	var cpus []int
	for _, g := range t.CoreGroups {
		if g.IsPCore() {
			cpus = append(cpus, g.AllCPUs...)
		}
	}
	return cpus
}

func (t *CPUTopology) GetAllECoresVCPUs() []int {
	var cpus []int
	for _, g := range t.CoreGroups {
		if g.IsECore() {
			cpus = append(cpus, g.AllCPUs...)
		}
	}
	return cpus
}

func (t *CPUTopology) TotalPCores() int {
	count := 0
	for _, g := range t.CoreGroups {
		if g.IsPCore() {
			count += len(g.PhysicalCPUs)
		}
	}
	return count
}

func (t *CPUTopology) TotalECores() int {
	count := 0
	for _, g := range t.CoreGroups {
		if g.IsECore() {
			count += len(g.PhysicalCPUs)
		}
	}
	return count
}
