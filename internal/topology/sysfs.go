package topology

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const SysfsBasePath = "/sys/devices/system/cpu"

func ReadIntFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return 0, errors.New("empty file")
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func ReadListFile(path string) ([]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return []int{}, nil
	}

	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if strings.Contains(item, "-") {
			bounds := strings.SplitN(item, "-", 2)
			if len(bounds) != 2 {
				return nil, errors.New("invalid range")
			}
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, err
			}
			if end < start {
				return nil, errors.New("range end before start")
			}
			for i := start; i <= end; i++ {
				values = append(values, i)
			}
			continue
		}
		parsed, err := strconv.Atoi(item)
		if err != nil {
			return nil, err
		}
		values = append(values, parsed)
	}

	sort.Ints(values)
	return dedupeSorted(values), nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ListCPUs() ([]int, error) {
	entries, err := os.ReadDir(SysfsBasePath)
	if err != nil {
		return nil, err
	}

	cpus := make([]int, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "cpu") {
			continue
		}
		suffix := strings.TrimPrefix(name, "cpu")
		if suffix == "" {
			continue
		}
		if strings.ContainsAny(suffix, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			continue
		}
		id, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		cpus = append(cpus, id)
	}

	sort.Ints(cpus)
	return cpus, nil
}

func cpuPath(cpuID int, element string) string {
	return filepath.Join(SysfsBasePath, "cpu"+strconv.Itoa(cpuID), "topology", element)
}

// cpuCachePath returns path to CPU cache info
// e.g., /sys/devices/system/cpu/cpu0/cache/index3/id
func cpuCachePath(cpuID int, index int, element string) string {
	return filepath.Join(SysfsBasePath, "cpu"+strconv.Itoa(cpuID), "cache", "index"+strconv.Itoa(index), element)
}

// ReadL3CacheID reads the L3 cache ID for a CPU
// L3 cache is typically index3, shared by cores in the same CCD
func ReadL3CacheID(cpuID int) (int, error) {
	// First, find which cache index is L3
	cacheBase := filepath.Join(SysfsBasePath, "cpu"+strconv.Itoa(cpuID), "cache")
	entries, err := os.ReadDir(cacheBase)
	if err != nil {
		return -1, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "index") {
			continue
		}

		// Check cache level
		levelPath := filepath.Join(cacheBase, entry.Name(), "level")
		level, err := ReadIntFile(levelPath)
		if err != nil {
			continue
		}

		// L3 cache is level 3
		if level == 3 {
			idPath := filepath.Join(cacheBase, entry.Name(), "id")
			return ReadIntFile(idPath)
		}
	}

	return -1, errors.New("L3 cache not found")
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
