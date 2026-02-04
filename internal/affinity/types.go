package affinity

import "epyc-pve/internal/topology"

type Strategy string

const (
	StrategySingleCCD   Strategy = "single-ccd"
	StrategyDistributed Strategy = "distributed"
	StrategySequential  Strategy = "sequential"
	StrategyRandom      Strategy = "random"
	StrategyManual      Strategy = "manual"
)

type Option struct {
	Strategy    Strategy
	Name        string
	Description string
	CPUs        []int
	AffinityStr string
	CCDsUsed    int
}

type Request struct {
	CoresNeeded int
	IncludeSMT  bool
	Topology    *topology.CPUTopology
}
