# Proxmox VE CPU Affinity Tool

A TUI tool for generating optimal CPU affinity configurations on Proxmox VE hosts. Automatically detects CPU topology (AMD CCDs, Intel hybrid P/E-cores) and suggests pinning strategies.

## Install

```bash
curl -fsSL https://github.com/ChanningHe/proxmox-affinity/releases/download/latest/proxmox-affinity-linux-amd64 -o proxmox-affinity
chmod +x proxmox-affinity
```

## Usage

```bash
./proxmox-affinity
```

Interactive TUI guides you through:
1. Select allocation type (physical cores or vCPUs)
2. Enter number of cores needed
3. Choose affinity strategy
4. Copy result or apply directly to a VM

### AMD (EPYC/Ryzen)
- **Single CCD** - Best cache locality
- **Distributed** - Spread across CCDs
- **Sequential** - First N cores
- **Manual** - Select CCDs manually

### Intel Hybrid (12th gen+)
**!!NOT TESTED!!**
- **P-Cores Only** - Performance cores
- **E-Cores Only** - Efficiency cores
- **All Cores** - Mixed

## Requirements

- Proxmox VE host (Linux with sysfs)
- `qm` command available for VM affinity application
