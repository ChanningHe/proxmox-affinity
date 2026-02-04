package pve

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type VM struct {
	VMID   int
	Name   string
	Status string
}

var ErrPermissionDenied = errors.New("permission denied")
var ErrVMNotFound = errors.New("vm not found")

func ListVMs() ([]VM, error) {
	cmd := exec.Command("qm", "list")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, wrapCommandError(err, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	vms := make([]VM, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "VMID") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}
		vmid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		vms = append(vms, VM{
			VMID:   vmid,
			Name:   fields[1],
			Status: fields[2],
		})
	}

	sort.Slice(vms, func(i, j int) bool {
		return vms[i].VMID < vms[j].VMID
	})

	return vms, nil
}

func SetAffinity(vmid int, affinity string, dryRun bool) error {
	if vmid <= 0 {
		return errors.New("vmid must be greater than zero")
	}
	if strings.TrimSpace(affinity) == "" {
		return errors.New("affinity string is empty")
	}
	if dryRun {
		return nil
	}

	cmd := exec.Command("qm", "set", strconv.Itoa(vmid), "--affinity", affinity)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return wrapCommandError(err, stderr.String())
	}
	return nil
}

func VMExists(vmid int) (bool, error) {
	vms, err := ListVMs()
	if err != nil {
		return false, err
	}
	for _, vm := range vms {
		if vm.VMID == vmid {
			return true, nil
		}
	}
	return false, nil
}

func wrapCommandError(err error, stderr string) error {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w: %v", ErrPermissionDenied, err)
	}
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "permission denied") {
		return fmt.Errorf("%w: %s", ErrPermissionDenied, strings.TrimSpace(stderr))
	}
	if strings.TrimSpace(stderr) != "" {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr))
	}
	return err
}
