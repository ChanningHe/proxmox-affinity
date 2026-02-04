package cmd

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"epyc-pve/internal/affinity"
	"epyc-pve/internal/topology"
)

type Options struct {
	ShowTopology bool
	Cores        int
	VMID         int
	Strategy     string
	Apply        bool
	DryRun       bool
	Physical     bool
	JSON         bool
}

var ErrInvalidArguments = errors.New("invalid arguments")

func ParseFlags() *Options {
	opts := &Options{}
	flag.BoolVar(&opts.ShowTopology, "topology", false, "Show CPU topology and exit")
	flag.IntVar(&opts.Cores, "cores", 0, "Number of cores/vCPUs to allocate")
	flag.IntVar(&opts.VMID, "vmid", 0, "Target VM ID")
	flag.StringVar(&opts.Strategy, "strategy", "", "Strategy: single-ccd, distributed, sequential, random")
	flag.BoolVar(&opts.Apply, "apply", false, "Apply affinity in CLI mode (non-interactive)")
	flag.BoolVar(&opts.DryRun, "dry-run", false, "Show command without executing")
	flag.BoolVar(&opts.Physical, "physical", false, "Use physical cores only (no SMT siblings)")
	flag.BoolVar(&opts.JSON, "json", false, "Output in JSON format (with --topology)")
	flag.Parse()
	return opts
}

func Validate(opts *Options, topo *topology.CPUTopology) error {
	if opts == nil {
		return fmt.Errorf("%w: options are required", ErrInvalidArguments)
	}

	if opts.ShowTopology && opts.Apply {
		return fmt.Errorf("%w: --topology cannot be used with --apply", ErrInvalidArguments)
	}
	if opts.JSON && !opts.ShowTopology {
		return fmt.Errorf("%w: --json requires --topology", ErrInvalidArguments)
	}
	if opts.DryRun && !opts.Apply {
		return fmt.Errorf("%w: --dry-run requires --apply", ErrInvalidArguments)
	}

	if opts.Apply {
		if opts.Cores <= 0 {
			return fmt.Errorf("%w: --cores is required for --apply mode", ErrInvalidArguments)
		}
		if opts.VMID <= 0 {
			return fmt.Errorf("%w: --vmid is required for --apply mode", ErrInvalidArguments)
		}

		maxCores := topo.TotalCores
		if !opts.Physical {
			maxCores = topo.TotalCPUs
		}
		if opts.Cores > maxCores {
			coreType := "vCPUs"
			if opts.Physical {
				coreType = "physical cores"
			}
			return fmt.Errorf("%w: requested %d %s, but only %d available",
				ErrInvalidArguments, opts.Cores, coreType, maxCores)
		}

		if opts.Strategy != "" {
			normalized := strings.ToLower(strings.TrimSpace(opts.Strategy))
			switch normalized {
			case string(affinity.StrategySingleCCD), string(affinity.StrategyDistributed),
				string(affinity.StrategySequential), string(affinity.StrategyRandom):
				opts.Strategy = normalized
			default:
				return fmt.Errorf("%w: invalid strategy %q (valid: single-ccd, distributed, sequential, random)",
					ErrInvalidArguments, opts.Strategy)
			}
		}
		return nil
	}

	if opts.Cores != 0 || opts.VMID != 0 || opts.Strategy != "" || opts.Physical || opts.DryRun {
		return fmt.Errorf("%w: use --apply for CLI mode, or run without flags for interactive mode", ErrInvalidArguments)
	}

	return nil
}
