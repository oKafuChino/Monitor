//go:build linux

package diskio

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixtureSource(t *testing.T) *linuxSource {
	t.Helper()
	root := t.TempDir()
	s := &linuxSource{proc: filepath.Join(root, "proc"), sys: filepath.Join(root, "sys")}
	if err := os.MkdirAll(s.proc, 0755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(s.sys, "class", "block"), 0755); err != nil { t.Fatal(err) }
	return s
}

func addDevice(t *testing.T, s *linuxSource, name, id, parent string, holders, slaves []string) {
	t.Helper()
	path := filepath.Join(s.sys, "devices", name)
	if parent != "" { path = filepath.Join(s.sys, "devices", parent, name) }
	for _, relation := range []string{"holders", "slaves"} { if err := os.MkdirAll(filepath.Join(path, relation), 0755); err != nil { t.Fatal(err) } }
	if err := os.WriteFile(filepath.Join(path, "dev"), []byte(id), 0644); err != nil { t.Fatal(err) }
	if parent != "" { if err := os.WriteFile(filepath.Join(path, "partition"), []byte("1"), 0644); err != nil { t.Fatal(err) } }
	if err := os.Symlink(path, filepath.Join(s.sys, "class", "block", name)); err != nil { t.Fatal(err) }
	for _, name := range holders { if err := os.WriteFile(filepath.Join(path, "holders", name), nil, 0644); err != nil { t.Fatal(err) } }
	for _, name := range slaves { if err := os.WriteFile(filepath.Join(path, "slaves", name), nil, 0644); err != nil { t.Fatal(err) } }
}

func writeStats(t *testing.T, s *linuxSource, names []string) {
	t.Helper(); var rows strings.Builder
	for _, name := range names {
		d := s.devices[name]; id := strings.SplitN(d.identity, ":", 3)
		fmt.Fprintf(&rows, "%s %s %s 0 0 4096 0 0 0 2048 0 0 0 0\n", id[0], id[1], name)
	}
	if err := os.WriteFile(filepath.Join(s.proc, "diskstats"), []byte(rows.String()), 0644); err != nil { t.Fatal(err) }
}

func TestTopologySelectionAndSectorUnits(t *testing.T) {
	s := fixtureSource(t)
	addDevice(t, s, "sda", "8:0", "", nil, nil)
	addDevice(t, s, "sda1", "8:1", "sda", []string{"dm-0"}, nil)
	if err := os.RemoveAll(filepath.Join(s.sys, "devices", "sda", "sda1", "slaves")); err != nil { t.Fatal(err) }
	addDevice(t, s, "dm-0", "253:0", "", nil, []string{"sda1"})
	addDevice(t, s, "nvme0n1", "259:0", "", []string{"md0"}, nil)
	addDevice(t, s, "vda", "252:0", "", []string{"md0"}, nil)
	addDevice(t, s, "md0", "9:0", "", nil, []string{"nvme0n1", "vda"})
	addDevice(t, s, "vdb", "252:16", "", nil, nil)
	addDevice(t, s, "loop0", "7:0", "", nil, nil)
	addDevice(t, s, "ram0", "1:0", "", nil, nil)
	addDevice(t, s, "zram0", "254:0", "", nil, nil)
	now := time.Now()
	if err := s.refresh(now); err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(s.selected, []string{"dm-0", "md0", "vdb"}) { t.Fatalf("double-counted topology: %v", s.selected) }
	writeStats(t, s, []string{"sda", "sda1", "dm-0", "nvme0n1", "vda", "md0", "vdb"})
	values, err := s.Read(now)
	if err != nil || len(values) != 3 { t.Fatalf("counters: %v %v", values, err) }
	for _, value := range values { if value.Read != 2<<20 || value.Write != 1<<20 { t.Fatalf("wrong sector units: %+v", value) } }
	for _, names := range [][]string{{"sda", "dm-0"}, {"vda", "md0"}, {"sda1"}, {"loop0"}, {"missing"}} {
		s.explicit = names; if err := s.refresh(now); err == nil { t.Fatalf("accepted overlapping/invalid devices %v", names) }
	}
	s.explicit = []string{"sda"}; if err := s.refresh(now); err != nil { t.Fatal(err) }
	writeStats(t, s, nil)
	if _, err := s.Read(now); err == nil { t.Fatal("missing device was silently treated as idle") }
}

func TestTopologyFailureAndRefresh(t *testing.T) {
	s := fixtureSource(t); addDevice(t, s, "vda", "252:0", "", nil, nil)
	now := time.Now(); if err := s.refresh(now); err != nil { t.Fatal(err) }; writeStats(t, s, []string{"vda"})
	before, err := s.Read(now); if err != nil { t.Fatal(err) }
	addDevice(t, s, "vdb", "252:16", "", nil, nil)
	if err := s.refresh(now.Add(time.Minute)); err != nil { t.Fatal(err) }; writeStats(t, s, []string{"vda", "vdb"})
	after, err := s.Read(now.Add(time.Minute)); if err != nil || len(after) != 2 { t.Fatal("refresh missed hotplug") }
	for id := range before { if _, exists := after[id]; exists { t.Fatal("topology identity did not change") } }
	if err := os.RemoveAll(filepath.Join(s.sys, "devices", "vda", "holders")); err != nil { t.Fatal(err) }
	if _, err := s.Read(now.Add(2*time.Minute)); err == nil { t.Fatal("unreadable topology did not fail closed") }
}
