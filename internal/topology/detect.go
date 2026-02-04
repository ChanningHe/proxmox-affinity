package topology

import (
	"errors"
	"fmt"
	"os"
	"sort"
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

	totalCPUs := len(infos)
	totalCores := 0
	for _, info := range infos {
		if info.IsFirstThread {
			totalCores++
		}
	}
	hasSMT := totalCPUs > totalCores

	method := detectCCDMethod(infos)
	ccds := groupByCCD(infos, method)

	packageMap := make(map[int][]CCD)
	for _, ccd := range ccds {
		packageMap[ccd.PackageID] = append(packageMap[ccd.PackageID], ccd)
	}

	packageIDs := make([]int, 0, len(packageMap))
	for id := range packageMap {
		packageIDs = append(packageIDs, id)
	}
	sort.Ints(packageIDs)

	packages := make([]Package, 0, len(packageIDs))
	for _, id := range packageIDs {
		packageCCDs := packageMap[id]
		sort.Slice(packageCCDs, func(i, j int) bool {
			return packageCCDs[i].ID < packageCCDs[j].ID
		})
		packages = append(packages, Package{ID: id, CCDs: packageCCDs})
	}

	return &CPUTopology{
		TotalCPUs:    totalCPUs,
		TotalCores:   totalCores,
		HasSMT:       hasSMT,
		Packages:     packages,
		CCDs:         ccds,
		DetectMethod: method,
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

	// Read L3 cache ID - this is the most accurate way to detect CCD
	l3CacheID, _ := ReadL3CacheID(cpuID)

	siblings, err := readOptionalList(cpuPath(cpuID, "thread_siblings_list"), []int{cpuID})
	if err != nil {
		return nil, err
	}
	sort.Ints(siblings)
	siblings = dedupeSorted(siblings)

	info := &CPUInfo{
		ID:             cpuID,
		PackageID:      packageID,
		CoreID:         coreID,
		ClusterID:      clusterID,
		DieID:          dieID,
		L3CacheID:      l3CacheID,
		ThreadSiblings: siblings,
		IsFirstThread:  len(siblings) == 0 || cpuID == siblings[0],
	}
	return info, nil
}

func groupByCCD(cpus []CPUInfo, method string) []CCD {
	type key struct {
		pkgID int
		ccdID int
	}
	groups := make(map[key]*CCD)

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
			// Inferred: group by core_id ranges
			if defaultCoresPerCCD > 0 {
				ccdID = cpu.CoreID / defaultCoresPerCCD
			}
		}

		groupKey := key{pkgID: cpu.PackageID, ccdID: ccdID}
		ccd, exists := groups[groupKey]
		if !exists {
			ccd = &CCD{
				ID:        ccdID,
				PackageID: cpu.PackageID,
				L3CacheID: l3ID,
			}
			groups[groupKey] = ccd
		}
		ccd.AllCPUs = append(ccd.AllCPUs, cpu.ID)
		if cpu.IsFirstThread {
			ccd.PhysicalCPUs = append(ccd.PhysicalCPUs, cpu.ID)
		}
	}

	list := make([]CCD, 0, len(groups))
	for _, ccd := range groups {
		sort.Ints(ccd.AllCPUs)
		ccd.AllCPUs = dedupeSorted(ccd.AllCPUs)
		sort.Ints(ccd.PhysicalCPUs)
		ccd.PhysicalCPUs = dedupeSorted(ccd.PhysicalCPUs)
		list = append(list, *ccd)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].PackageID == list[j].PackageID {
			return list[i].ID < list[j].ID
		}
		return list[i].PackageID < list[j].PackageID
	})

	// Re-number CCDs sequentially per package for cleaner display
	pkgCCDCount := make(map[int]int)
	for i := range list {
		list[i].ID = pkgCCDCount[list[i].PackageID]
		pkgCCDCount[list[i].PackageID]++
	}

	return list
}

func detectCCDMethod(cpus []CPUInfo) string {
	if len(cpus) == 0 {
		return "inferred"
	}

	// Priority 1: L3 Cache ID (most accurate for CCD detection)
	hasL3 := true
	l3Values := make(map[int]bool)
	for _, cpu := range cpus {
		if cpu.L3CacheID < 0 {
			hasL3 = false
			break
		}
		l3Values[cpu.L3CacheID] = true
	}
	// Only use L3 if we have multiple distinct L3 caches (indicates multiple CCDs)
	if hasL3 && len(l3Values) > 1 {
		return "l3_cache"
	}

	// Priority 2: cluster_id
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

	// Priority 3: die_id
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
