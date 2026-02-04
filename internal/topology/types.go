package topology

type CPUTopology struct {
	TotalCPUs    int       `json:"total_cpus"`
	TotalCores   int       `json:"total_cores"`
	HasSMT       bool      `json:"has_smt"`
	Packages     []Package `json:"packages"`
	CCDs         []CCD     `json:"ccds"`
	DetectMethod string    `json:"detect_method"` // "l3_cache", "cluster_id", "die_id", "inferred"
}

type Package struct {
	ID   int   `json:"id"`
	CCDs []CCD `json:"ccds"`
}

type CCD struct {
	ID           int   `json:"id"`
	PackageID    int   `json:"package_id"`
	L3CacheID    int   `json:"l3_cache_id"`   // The actual L3 cache ID
	PhysicalCPUs []int `json:"physical_cpus"` // First thread of each core
	AllCPUs      []int `json:"all_cpus"`      // All threads including SMT
}

type CPUInfo struct {
	ID             int
	PackageID      int
	CoreID         int
	ClusterID      int // -1 if not available
	DieID          int // -1 if not available
	L3CacheID      int // -1 if not available (KEY for CCD detection)
	ThreadSiblings []int
	IsFirstThread  bool
}
