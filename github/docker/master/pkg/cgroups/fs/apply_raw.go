package fs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dotcloud/docker/pkg/cgroups"
)

var (
	subsystems = []subsystem{
		&devicesGroup{},
		&memoryGroup{},
		&cpuGroup{},
		&cpusetGroup{},
		&cpuacctGroup{},
		&blkioGroup{},
		&perfEventGroup{},
		&freezerGroup{},
	}
)

type subsystem interface {
	Set(*data) error
}

type data struct {
	root   string
	cgroup string
	c      *cgroups.Cgroup
	pid    int
}

func Apply(c *cgroups.Cgroup, pid int) (cgroups.ActiveCgroup, error) {
	// We have two implementation of cgroups support, one is based on
	// systemd and the dbus api, and one is based on raw cgroup fs operations
	// following the pre-single-writer model docs at:
	// http://www.freedesktop.org/wiki/Software/systemd/PaxControlGroups/
	//
	// we can pick any subsystem to find the root

	cgroupRoot, err := cgroups.FindCgroupMountpoint("cpu")
	if err != nil {
		return nil, err
	}
	cgroupRoot = filepath.Dir(cgroupRoot)

	if _, err := os.Stat(cgroupRoot); err != nil {
		return nil, fmt.Errorf("cgroups fs not found")
	}

	cgroup := c.Name
	if c.Parent != "" {
		cgroup = filepath.Join(c.Parent, cgroup)
	}

	d := &data{
		root:   cgroupRoot,
		cgroup: cgroup,
		c:      c,
		pid:    pid,
	}
	for _, sys := range subsystems {
		if err := sys.Set(d); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (raw *data) path(subsystem string) (string, error) {
	initPath, err := cgroups.GetInitCgroupDir(subsystem)
	if err != nil {
		return "", err
	}
	return filepath.Join(raw.root, subsystem, initPath, raw.cgroup), nil
}

func (raw *data) join(subsystem string) (string, error) {
	path, err := raw.path(subsystem)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err := writeFile(path, "cgroup.procs", strconv.Itoa(raw.pid)); err != nil {
		return "", err
	}
	return path, nil
}

func (raw *data) Cleanup() error {
	get := func(subsystem string) string {
		path, _ := raw.path(subsystem)
		return path
	}

	for _, path := range []string{
		get("memory"),
		get("devices"),
		get("cpu"),
		get("cpuset"),
		get("cpuacct"),
		get("blkio"),
		get("perf_event"),
		get("freezer"),
	} {
		if path != "" {
			os.RemoveAll(path)
		}
	}
	return nil
}

func writeFile(dir, file, data string) error {
	return ioutil.WriteFile(filepath.Join(dir, file), []byte(data), 0700)
}
