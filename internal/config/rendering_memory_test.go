package config

import "testing"

func TestRenderingChargedMemoryContract(t *testing.T) {
	c := DefaultRendering()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Enabled = true
	c.WorkerPath = "/opt/chartworks/renderer"
	if c.Validate() == nil {
		t.Fatal("missing cgroup delegation admitted")
	}
	c.CgroupRoot = "/sys/fs/cgroup/chartworks-render"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{0, 31 << 20, (1 << 30) + 1, 2 << 30} {
		bad := c
		bad.MaxMemoryBytes = n
		if bad.Validate() == nil {
			t.Fatal("budget admitted", n)
		}
	}
	c.CgroupRoot = "relative"
	if c.Validate() == nil {
		t.Fatal("relative delegation admitted")
	}
}
