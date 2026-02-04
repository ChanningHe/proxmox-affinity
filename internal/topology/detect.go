package topology

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

var ErrTopologyUnavailable = errors.New("topology unavailable")

const defaultCoresPerCCD = 8

func Detect() (*CPUTopology, error) {
	info, err := os.Stat(SysfsBasePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: sysfs base path not found", ErrTopologyUnavailable)
		}
		if os.IsPermission(err) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrTopologyUnavailable, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: sysfs base path not a directory", ErrTopologyUnavailable)
	}

	cpuIDs, err := ListCPUs()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTopologyUnavailable, err)
	}
	if len(cpuIDs) == 0 {
		return nil, fmt.Errorf("%w: no CPUs found", ErrTopologyUnavailable)
	}

	infos := make([]CPUInfo, 0, len(cpuIDs))
	for _, id := range cpuIDs {
		info, err := readCPUInfo(id)
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil, err
			}
			return nil, fmt.Errorf("%w: %v", ErrTopologyUnavailable, err)
		}
		infos = append(infos, *info)
	}

	arch := detectArchitecture(infos)

	switch arch {
	case ArchIntelHybrid:
		return buildIntelHybridTopology(infos)
	case ArchAMD:
		return buildAMDTopology(infos)
	default:
		return buildGenericTopology(infos)
	}
}

func detectArchitecture(cpus []CPUInfo) Architecture {
	vendor := readCPUVendor()

	switch vendor {
	case "AuthenticAMD", "AMD":
		return ArchAMD
	case "GenuineIntel":
		if hasHybridCores(cpus) {
			return ArchIntelHybrid
		}
		return ArchGeneric
	default:
		if hasMultipleL3(cpus) {
			return ArchAMD
		}
		return ArchGeneric
	}
}

func readCPUVendor() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "vendor_id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func hasHybridCores(cpus []CPUInfo) bool {
	capacities := make(map[int]bool)
	for _, cpu := range cpus {
		if cpu.Capacity > 0 {
			capacities[cpu.Capacity] = true
		}
	}
	return len(capacities) > 1
}

func hasMultipleL3(cpus []CPUInfo) bool {
	l3Values := make(map[int]bool)
	for _, cpu := range cpus {
		if cpu.L3CacheID >= 0 {
			l3Values[cpu.L3CacheID] = true
		}
	}
	return len(l3Values) > 1
}

func buildAMDTopology(infos []CPUInfo) (*CPUTopology, error) {
	totalCPUs := len(infos)
	totalCores := 0
	for _, info := range infos {
		if info.IsFirstThread {
			totalCores++
		}
	}
	hasSMT := totalCPUs > totalCores

	method := detectCCDMethod(infos)
	coreGroups := groupByCCD(infos, method)

	packageMap := make(map[int][]CoreGroup)
	for _, cg := range coreGroups {
		packageMap[cg.PackageID] = append(packageMap[cg.PackageID], cg)
	}

	packageIDs := make([]int, 0, len(packageMap))
	for id := range packageMap {
		packageIDs = append(packageIDs, id)
	}
	sort.Ints(packageIDs)

	packages := make([]Package, 0, len(packageIDs))
	for _, id := range packageIDs {
		pkgGroups := packageMap[id]
		sort.Slice(pkgGroups, func(i, j int) bool {
			return pkgGroups[i].ID < pkgGroups[j].ID
		})
		packages = append(packages, Package{ID: id, CoreGroups: pkgGroups})
	}

	return &CPUTopology{
		Architecture: ArchAMD,
		TotalCPUs:    totalCPUs,
		TotalCores:   totalCores,
		HasSMT:       hasSMT,
		Packages:     packages,
		CoreGroups:   coreGroups,
		DetectMethod: method,
	}, nil
}

func buildIntelHybridTopology(infos []CPUInfo) (*CPUTopology, error) {
	totalCPUs := len(infos)
	totalCores := 0
	for _, info := range infos {
		if info.IsFirstThread {
			totalCores++
		}
	}
	hasSMT := totalCPUs > totalCores

	coreGroups := groupByIntelCoreType(infos)

	packageMap := make(map[int][]CoreGroup)
	for _, cg := range coreGroups {
		packageMap[cg.PackageID] = append(packageMap[cg.PackageID], cg)
	}

	packageIDs := make([]int, 0, len(packageMap))
	for id := range packageMap {
		packageIDs = append(packageIDs, id)
	}
	sort.Ints(packageIDs)

	packages := make([]Package, 0, len(packageIDs))
	for _, id := range packageIDs {
		pkgGroups := packageMap[id]
		sort.Slice(pkgGroups, func(i, j int) bool {
			if pkgGroups[i].Type != pkgGroups[j].Type {
				return pkgGroups[i].Type == CoreTypePerformance
			}
			return pkgGroups[i].ID < pkgGroups[j].ID
		})
		packages = append(packages, Package{ID: id, CoreGroups: pkgGroups})
	}

	return &CPUTopology{
		Architecture: ArchIntelHybrid,
		TotalCPUs:    totalCPUs,
		TotalCores:   totalCores,
		HasSMT:       hasSMT,
		Packages:     packages,
		CoreGroups:   coreGroups,
		DetectMethod: "intel_hybrid",
	}, nil
}

func buildGenericTopology(infos []CPUInfo) (*CPUTopology, error) {
	totalCPUs := len(infos)
	totalCores := 0
	for _, info := range infos {
		if info.IsFirstThread {
			totalCores++
		}
	}
	hasSMT := totalCPUs > totalCores

	coreGroup := CoreGroup{
		ID:        0,
		PackageID: 0,
		Type:      CoreTypeUnknown,
		Name:      "All Cores",
		L3CacheID: -1,
	}

	for _, info := range infos {
		coreGroup.AllCPUs = append(coreGroup.AllCPUs, info.ID)
		if info.IsFirstThread {
			coreGroup.PhysicalCPUs = append(coreGroup.PhysicalCPUs, info.ID)
		}
	}
	sort.Ints(coreGroup.AllCPUs)
	sort.Ints(coreGroup.PhysicalCPUs)

	coreGroups := []CoreGroup{coreGroup}
	packages := []Package{{ID: 0, CoreGroups: coreGroups}}

	return &CPUTopology{
		Architecture: ArchGeneric,
		TotalCPUs:    totalCPUs,
		TotalCores:   totalCores,
		HasSMT:       hasSMT,
		Packages:     packages,
		CoreGroups:   coreGroups,
		DetectMethod: "generic",
	}, nil
}

func readCPUInfo(cpuID int) (*CPUInfo, error) {
	packageID, err := readOptionalInt(cpuPath(cpuID, "physical_package_id"), 0)
	if err != nil {
		return nil, err
	}
	coreID, err := readOptionalInt(cpuPath(cpuID, "core_id"), cpuID)
	if err != nil {
		return nil, err
	}
	clusterID, err := readOptionalInt(cpuPath(cpuID, "cluster_id"), -1)
	if err != nil {
		return nil, err
	}
	dieID, err := readOptionalInt(cpuPath(cpuID, "die_id"), -1)
	if err != nil {
		return nil, err
	}

	l3CacheID, _ := ReadL3CacheID(cpuID)

	siblings, err := readOptionalList(cpuPath(cpuID, "thread_siblings_list"), []int{cpuID})
	if err != nil {
		return nil, err
	}
	sort.Ints(siblings)
	siblings = dedupeSorted(siblings)

	capacity := readCPUCapacity(cpuID)
	coreType := detectCoreType(cpuID, capacity, siblings)

	info := &CPUInfo{
		ID:             cpuID,
		PackageID:      packageID,
		CoreID:         coreID,
		ClusterID:      clusterID,
		DieID:          dieID,
		L3CacheID:      l3CacheID,
		ThreadSiblings: siblings,
		IsFirstThread:  len(siblings) == 0 || cpuID == siblings[0],
		CoreType:       coreType,
		Capacity:       capacity,
	}
	return info, nil
}

func readCPUCapacity(cpuID int) int {
	path := cpuCapacityPath(cpuID)
	value, err := ReadIntFile(path)
	if err != nil {
		return 0
	}
	return value
}

func detectCoreType(cpuID int, capacity int, siblings []int) CoreType {
	if capacity >= 1000 {
		return CoreTypePerformance
	}
	if capacity > 0 && capacity < 900 {
		return CoreTypeEfficiency
	}

	return CoreTypeUnknown
}

func groupByIntelCoreType(cpus []CPUInfo) []CoreGroup {
	pCores := CoreGroup{
		ID:        0,
		Type:      CoreTypePerformance,
		Name:      "P-Cores",
		L3CacheID: -1,
	}
	eCores := CoreGroup{
		ID:        1,
		Type:      CoreTypeEfficiency,
		Name:      "E-Cores",
		L3CacheID: -1,
	}

	for _, cpu := range cpus {
		if cpu.PackageID > pCores.PackageID {
			pCores.PackageID = cpu.PackageID
		}
		if cpu.PackageID > eCores.PackageID {
			eCores.PackageID = cpu.PackageID
		}

		if cpu.CoreType == CoreTypePerformance {
			pCores.AllCPUs = append(pCores.AllCPUs, cpu.ID)
			if cpu.IsFirstThread {
				pCores.PhysicalCPUs = append(pCores.PhysicalCPUs, cpu.ID)
			}
		} else if cpu.CoreType == CoreTypeEfficiency {
			eCores.AllCPUs = append(eCores.AllCPUs, cpu.ID)
			if cpu.IsFirstThread {
				eCores.PhysicalCPUs = append(eCores.PhysicalCPUs, cpu.ID)
			}
		} else {
			if len(cpu.ThreadSiblings) > 1 {
				pCores.AllCPUs = append(pCores.AllCPUs, cpu.ID)
				if cpu.IsFirstThread {
					pCores.PhysicalCPUs = append(pCores.PhysicalCPUs, cpu.ID)
				}
			} else {
				eCores.AllCPUs = append(eCores.AllCPUs, cpu.ID)
				if cpu.IsFirstThread {
					eCores.PhysicalCPUs = append(eCores.PhysicalCPUs, cpu.ID)
				}
			}
		}
	}

	sort.Ints(pCores.AllCPUs)
	sort.Ints(pCores.PhysicalCPUs)
	sort.Ints(eCores.AllCPUs)
	sort.Ints(eCores.PhysicalCPUs)

	var groups []CoreGroup
	if len(pCores.PhysicalCPUs) > 0 {
		groups = append(groups, pCores)
	}
	if len(eCores.PhysicalCPUs) > 0 {
		groups = append(groups, eCores)
	}

	return groups
}

func groupByCCD(cpus []CPUInfo, method string) []CoreGroup {
	type key struct {
		pkgID int
		ccdID int
	}
	groups := make(map[key]*CoreGroup)

	for _, cpu := range cpus {
		ccdID := 0
		l3ID := cpu.L3CacheID

		switch method {
		case "l3_cache":
			ccdID = cpu.L3CacheID
		case "cluster_id":
			ccdID = cpu.ClusterID
		case "die_id":
			ccdID = cpu.DieID
		default:
			if defaultCoresPerCCD > 0 {
				ccdID = cpu.CoreID / defaultCoresPerCCD
			}
		}

		groupKey := key{pkgID: cpu.PackageID, ccdID: ccdID}
		cg, exists := groups[groupKey]
		if !exists {
			cg = &CoreGroup{
				ID:        ccdID,
				PackageID: cpu.PackageID,
				Type:      CoreTypeUnknown,
				Name:      fmt.Sprintf("CCD %d", ccdID),
				L3CacheID: l3ID,
			}
			groups[groupKey] = cg
		}
		cg.AllCPUs = append(cg.AllCPUs, cpu.ID)
		if cpu.IsFirstThread {
			cg.PhysicalCPUs = append(cg.PhysicalCPUs, cpu.ID)
		}
	}

	list := make([]CoreGroup, 0, len(groups))
	for _, cg := range groups {
		sort.Ints(cg.AllCPUs)
		cg.AllCPUs = dedupeSorted(cg.AllCPUs)
		sort.Ints(cg.PhysicalCPUs)
		cg.PhysicalCPUs = dedupeSorted(cg.PhysicalCPUs)
		list = append(list, *cg)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].PackageID == list[j].PackageID {
			return list[i].ID < list[j].ID
		}
		return list[i].PackageID < list[j].PackageID
	})

	pkgCCDCount := make(map[int]int)
	for i := range list {
		list[i].ID = pkgCCDCount[list[i].PackageID]
		list[i].Name = fmt.Sprintf("CCD %d", list[i].ID)
		pkgCCDCount[list[i].PackageID]++
	}

	return list
}

func detectCCDMethod(cpus []CPUInfo) string {
	if len(cpus) == 0 {
		return "inferred"
	}

	hasL3 := true
	l3Values := make(map[int]bool)
	for _, cpu := range cpus {
		if cpu.L3CacheID < 0 {
			hasL3 = false
			break
		}
		l3Values[cpu.L3CacheID] = true
	}
	if hasL3 && len(l3Values) > 1 {
		return "l3_cache"
	}

	hasCluster := true
	for _, cpu := range cpus {
		if cpu.ClusterID < 0 {
			hasCluster = false
			break
		}
	}
	if hasCluster {
		return "cluster_id"
	}

	hasDie := true
	for _, cpu := range cpus {
		if cpu.DieID < 0 {
			hasDie = false
			break
		}
	}
	if hasDie {
		return "die_id"
	}

	return "inferred"
}

func readOptionalInt(path string, defaultValue int) (int, error) {
	value, err := ReadIntFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultValue, nil
		}
		return 0, err
	}
	return value, nil
}

func readOptionalList(path string, defaultValue []int) ([]int, error) {
	values, err := ReadListFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			copyValue := make([]int, len(defaultValue))
			copy(copyValue, defaultValue)
			return copyValue, nil
		}
		return nil, err
	}
	return values, nil
}
