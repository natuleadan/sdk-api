package runtime

import (
	"os"
	"runtime/debug"
	"testing"
)

func TestInitGC_NilConfig(t *testing.T) {
	initGC(nil)
}

func TestInitGC_GOGC(t *testing.T) {
	prev := debug.SetGCPercent(100)
	initGC(&GCConfig{GOGC: 200})
	cur := debug.SetGCPercent(prev)
	if cur != 200 {
		t.Errorf("expected GOGC=200, got %d", cur)
	}
}

func TestParseBytes_Empty(t *testing.T) {
	if v := parseBytes(""); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestParseBytes_GiB(t *testing.T) {
	if v := parseBytes("2GiB"); v != 2<<30 {
		t.Errorf("expected %d, got %d", 2<<30, v)
	}
}

func TestParseBytes_MiB(t *testing.T) {
	if v := parseBytes("512MiB"); v != 512<<20 {
		t.Errorf("expected %d, got %d", 512<<20, v)
	}
}

func TestParseBytes_GB(t *testing.T) {
	if v := parseBytes("1GB"); v != 1000000000 {
		t.Errorf("expected 1000000000, got %d", v)
	}
}

func TestParseBytes_MB(t *testing.T) {
	if v := parseBytes("100MB"); v != 100000000 {
		t.Errorf("expected 100000000, got %d", v)
	}
}

func TestParseBytes_Invalid(t *testing.T) {
	if v := parseBytes("invalid"); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestParseMemoryLimit_Percentage_NoCgroup(t *testing.T) {
	v := parseMemoryLimit("50%")
	if v != 0 {
		t.Errorf("expected 0 (no cgroup), got %d", v)
	}
}

func TestParseMemoryLimit_Empty(t *testing.T) {
	if v := parseMemoryLimit(""); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestParseMemoryLimit_InvalidPct(t *testing.T) {
	if v := parseMemoryLimit("invalid%"); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestReadCgroupMemoryLimit(t *testing.T) {
	v := readCgroupMemoryLimit()
	if v < 0 {
		t.Errorf("expected >=0, got %d", v)
	}
}

func TestInitGC_MemoryLimit(t *testing.T) {
	prev := debug.SetMemoryLimit(1 << 30)
	initGC(&GCConfig{MemoryLimit: "512MiB"})
	cur := debug.SetMemoryLimit(prev)
	if cur != 512<<20 {
		t.Errorf("expected 512MiB limit, got %d", cur)
	}
}

func TestInitGC_MemoryLimitPercentage(t *testing.T) {
	os.WriteFile("/tmp/test_memory_max.txt", []byte("1073741824\n"), 0o644)
	defer os.Remove("/tmp/test_memory_max.txt")
	prev := debug.SetMemoryLimit(1 << 30)
	saved := cgroupMemoryLimitPath
	cgroupMemoryLimitPath = "/tmp/test_memory_max.txt"
	t.Cleanup(func() { cgroupMemoryLimitPath = saved })
	initGC(&GCConfig{MemoryLimit: "80%"})
	cur := debug.SetMemoryLimit(prev)
	if cur != 858993459 {
		t.Errorf("expected ~858993459 (80%% of 1073741824), got %d", cur)
	}
}

func TestParseBytes_Suffixes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1TiB", 1 << 40},
		{"1GiB", 1 << 30},
		{"1MiB", 1 << 20},
		{"1KiB", 1 << 10},
		{"1TB", 1e12},
		{"1GB", 1e9},
		{"1MB", 1e6},
		{"1KB", 1e3},
		{"1B", 1},
		{"100", 100},
	}
	for _, tt := range tests {
		if v := parseBytes(tt.input); v != tt.want {
			t.Errorf("parseBytes(%q) = %d, want %d", tt.input, v, tt.want)
		}
	}
}
