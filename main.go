package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"epyc-pve/cmd"
	"epyc-pve/internal/affinity"
	"epyc-pve/internal/pve"
	"epyc-pve/internal/topology"
	"epyc-pve/internal/ui"
)

func main() {
	opts := cmd.ParseFlags()

	topo, err := topology.Detect()
	if err != nil {
		exitWithError(err)
	}

	if err := cmd.Validate(opts, topo); err != nil {
		exitWithError(err)
	}

	if opts.ShowTopology {
		if opts.JSON {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(topo); err != nil {
				exitWithError(err)
			}
			return
		}
		ui.PrintTopology(topo)
		return
	}

	if opts.Apply {
		if err := runCLIMode(opts, topo); err != nil {
			exitWithError(err)
		}
		return
	}

	if err := ui.Run(topo); err != nil {
		exitWithError(err)
	}
}

func runCLIMode(opts *cmd.Options, topo *topology.CPUTopology) error {
	req := &affinity.Request{
		CoresNeeded: opts.Cores,
		IncludeSMT:  !opts.Physical,
		Topology:    topo,
	}
	options, err := affinity.Generate(req)
	if err != nil {
		return err
	}

	strategy := opts.Strategy
	if strings.TrimSpace(strategy) == "" {
		strategy = string(affinity.StrategyRandom)
	}

	selected, ok := selectOption(options, affinity.Strategy(strategy))
	if !ok {
		return fmt.Errorf("%w: invalid strategy %q", cmd.ErrInvalidArguments, strategy)
	}
	if len(selected.CPUs) == 0 {
		return fmt.Errorf("%w: %s", cmd.ErrInvalidArguments, selected.Description)
	}

	vms, err := pve.ListVMs()
	if err != nil {
		return err
	}
	if !vmIDExists(opts.VMID, vms) {
		return fmt.Errorf("%w: VM %d not found. Available VMs: %s", pve.ErrVMNotFound, opts.VMID, formatVMIDs(vms))
	}

	if opts.DryRun {
		ui.PrintDryRun(opts.VMID, selected.AffinityStr)
		return nil
	}

	if err := pve.SetAffinity(opts.VMID, selected.AffinityStr, false); err != nil {
		return err
	}
	ui.PrintSuccess(opts.VMID, selected.AffinityStr)
	return nil
}

func selectOption(options []affinity.Option, strategy affinity.Strategy) (affinity.Option, bool) {
	for _, option := range options {
		if option.Strategy == strategy {
			return option, true
		}
	}
	return affinity.Option{}, false
}

func vmIDExists(vmid int, vms []pve.VM) bool {
	for _, vm := range vms {
		if vm.VMID == vmid {
			return true
		}
	}
	return false
}

func formatVMIDs(vms []pve.VM) string {
	if len(vms) == 0 {
		return "none"
	}
	ids := make([]string, 0, len(vms))
	for _, vm := range vms {
		ids = append(ids, fmt.Sprintf("%d", vm.VMID))
	}
	return strings.Join(ids, ", ")
}

func exitWithError(err error) {
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, cmd.ErrInvalidArguments):
		ui.PrintError(err)
		os.Exit(2)
	case errors.Is(err, pve.ErrPermissionDenied) || errors.Is(err, os.ErrPermission):
		ui.PrintError(errors.New("Permission denied. Try running with sudo."))
		os.Exit(5)
	case errors.Is(err, pve.ErrVMNotFound):
		ui.PrintError(err)
		os.Exit(4)
	case errors.Is(err, topology.ErrTopologyUnavailable):
		ui.PrintError(errors.New("Cannot read CPU topology. Are you running on a Linux system?"))
		os.Exit(3)
	default:
		ui.PrintError(err)
		os.Exit(1)
	}
}
