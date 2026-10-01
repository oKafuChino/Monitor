//go:build linux

package diskio

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type blockDevice struct {
	name, identity, parent string
	partition              bool
	holders, slaves        []string
}

type linuxSource struct {
	proc, sys   string
	explicit    []string
	devices     map[string]blockDevice
	selected    []string
	refreshed   time.Time
	fingerprint string
}

func NewSource(devices []string) Source {
	proc := os.Getenv("HOST_PROC")
	if proc == "" {
		proc = "/proc"
	}
	sys := os.Getenv("HOST_SYS")
	if sys == "" {
		sys = "/sys"
	}
	return &linuxSource{proc: proc, sys: sys, explicit: devices}
}

// NewSourceAt also honors paths from the probe's JSON configuration.
func NewSourceAt(devices []string, proc, sys string) Source {
	s := NewSource(devices).(*linuxSource)
	if proc != "" {
		s.proc = proc
	}
	if sys != "" {
		s.sys = sys
	}
	return s
}

// ValidateConfiguration reports explicit device conflicts at startup. Auto mode
// may recover from temporarily missing kernel interfaces on later samples.
func ValidateConfiguration(source Source) error {
	s := source.(*linuxSource)
	if len(s.explicit) == 0 {
		return nil
	}
	return s.refresh(time.Now())
}

func excluded(name string) bool {
	return strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram")
}

func (s *linuxSource) refresh(now time.Time) error {
	root := filepath.Join(s.sys, "class", "block")
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	devices := map[string]blockDevice{}
	for _, entry := range entries {
		name := entry.Name()
		if excluded(name) {
			continue
		}
		path := filepath.Join(root, name)
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		dev, err := os.ReadFile(filepath.Join(path, "dev"))
		if err != nil {
			return err
		}
		d := blockDevice{name: name, identity: strings.TrimSpace(string(dev)) + ":" + resolved}
		_, err = os.Stat(filepath.Join(path, "partition"))
		if err == nil {
			d.partition = true
			d.parent = filepath.Base(filepath.Dir(resolved))
		} else if !os.IsNotExist(err) {
			return err
		}
		for _, relation := range []string{"holders", "slaves"} {
			// Partitions expose holders, but Linux does not require a slaves directory.
			if d.partition && relation == "slaves" {
				continue
			}
			items, err := os.ReadDir(filepath.Join(path, relation))
			if err != nil {
				return err
			}
			names := make([]string, 0, len(items))
			for _, item := range items {
				names = append(names, item.Name())
			}
			if relation == "holders" {
				d.holders = names
			} else {
				d.slaves = names
			}
		}
		devices[name] = d
	}
	// Edges run from lower to upper layers. A partition's parent contributes to
	// its counters, so parent -> partition is also an ancestry edge.
	edges := map[string][]string{}
	for name, d := range devices {
		if d.partition {
			if _, ok := devices[d.parent]; !ok {
				return fmt.Errorf("missing partition parent for %s", name)
			}
			edges[d.parent] = append(edges[d.parent], name)
		}
		for _, holder := range d.holders {
			if _, ok := devices[holder]; !ok {
				return fmt.Errorf("unknown holder %s", holder)
			}
			edges[name] = append(edges[name], holder)
		}
		for _, slave := range d.slaves {
			if _, ok := devices[slave]; !ok {
				return fmt.Errorf("unknown slave %s", slave)
			}
			edges[slave] = append(edges[slave], name)
		}
	}
	// Cycles or incomplete topology cannot safely define an accounting layer.
	state := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 1 {
			return fmt.Errorf("cyclic block topology")
		}
		if state[name] == 2 {
			return nil
		}
		state[name] = 1
		for _, upper := range edges[name] {
			if err := visit(upper); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	for name := range devices {
		if err := visit(name); err != nil {
			return err
		}
	}
	var reaches func(string, string) bool
	reaches = func(from, target string) bool {
		for _, upper := range edges[from] {
			if upper == target || reaches(upper, target) {
				return true
			}
		}
		return false
	}
	selected := append([]string(nil), s.explicit...)
	if len(selected) > 0 {
		for _, name := range selected {
			d, ok := devices[name]
			if !ok || d.partition {
				return fmt.Errorf("disk IO device %s is absent, excluded, or a partition", name)
			}
		}
		for i, name := range selected {
			for _, other := range selected[i+1:] {
				if reaches(name, other) || reaches(other, name) {
					return fmt.Errorf("disk IO devices %s and %s overlap", name, other)
				}
			}
		}
	} else {
		for name, d := range devices {
			if d.partition {
				continue
			}
			hasUpper := false
			for other, upper := range devices {
				if !upper.partition && other != name && reaches(name, other) {
					hasUpper = true
					break
				}
			}
			if !hasUpper {
				selected = append(selected, name)
			}
		}
	}
	sort.Strings(selected)
	names := make([]string, 0, len(devices))
	for name := range devices {
		names = append(names, name)
	}
	sort.Strings(names)
	var topology strings.Builder
	for _, name := range names {
		d := devices[name]
		fmt.Fprintf(&topology, "%s:%s:%s:%v:%v;", name, d.identity, d.parent, d.holders, d.slaves)
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(topology.String())))
	if fingerprint != s.fingerprint {
		log.Printf("Disk IO selected %d logical block devices: %v", len(selected), selected)
	}
	s.devices = devices
	s.selected = selected
	s.fingerprint = fingerprint
	s.refreshed = now
	return nil
}

func (s *linuxSource) Read(now time.Time) (Snapshot, error) {
	if s.devices == nil || now.Sub(s.refreshed) >= time.Minute || now.Before(s.refreshed) {
		if err := s.refresh(now); err != nil {
			s.devices = nil
			return nil, err
		}
	}
	result, err := s.readCounters()
	if err != nil {
		s.devices = nil
	}
	return result, err
}

func (s *linuxSource) readCounters() (Snapshot, error) {
	file, err := os.Open(filepath.Join(s.proc, "diskstats"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	wanted := map[string]bool{}
	for _, name := range s.selected {
		wanted[name] = true
	}
	result := Snapshot{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		name := fields[2]
		if !wanted[name] {
			continue
		}
		if len(fields) < 14 {
			return nil, fmt.Errorf("short diskstats row for %s", name)
		}
		d := s.devices[name]
		identity := fields[0] + ":" + fields[1]
		if !strings.HasPrefix(d.identity, identity+":") {
			return nil, fmt.Errorf("disk IO device changed")
		}
		read, err := strconv.ParseUint(fields[5], 10, 64)
		if err != nil {
			return nil, err
		}
		write, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			return nil, err
		}
		if read > math.MaxUint64/512 || write > math.MaxUint64/512 {
			return nil, fmt.Errorf("diskstats sector overflow")
		}
		// Linux diskstats sectors are always 512 bytes, irrespective of device size.
		result[d.identity+":"+s.fingerprint] = Counter{Read: read * 512, Write: write * 512}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) != len(wanted) {
		return nil, fmt.Errorf("disk IO device disappeared")
	}
	return result, nil
}
