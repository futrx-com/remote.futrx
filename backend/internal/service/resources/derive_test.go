package resources

import "testing"

func TestParseSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{name: "empty is zero", input: "", want: 0},
		{name: "whitespace is zero", input: "  ", want: 0},
		{name: "gibibytes", input: "2GiB", want: 2 * gib},
		{name: "mebibytes", input: "768MiB", want: 768 * mib},
		{name: "tebibytes", input: "1TiB", want: tib},
		{name: "kibibytes", input: "512KiB", want: 512 * kib},
		{name: "decimal family", input: "2GB", want: 2_000_000_000},
		{name: "bare bytes", input: "1048576", want: mib},
		{name: "explicit bytes suffix", input: "4096B", want: 4096},
		{name: "fractional gibibytes", input: "1.5GiB", want: gib + 512*mib},
		{name: "surrounding space", input: " 3GiB ", want: 3 * gib},
		{name: "unknown unit", input: "3 gigs", wantErr: true},
		{name: "negative", input: "-1GiB", wantErr: true},
		{name: "not a number", input: "GiB", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseSize(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("ParseSize(%q) = %d, want error", test.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSize(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("ParseSize(%q) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{bytes: 0, want: ""},
		{bytes: 2 * gib, want: "2GiB"},
		{bytes: 3 * gib, want: "3GiB"},
		{bytes: 768 * mib, want: "768MiB"},
		{bytes: 3*gib + 256*mib, want: "3328MiB"},
		{bytes: tib, want: "1TiB"},
		{bytes: 512, want: "512B"},
	}
	for _, test := range tests {
		if got := FormatSize(test.bytes); got != test.want {
			t.Fatalf("FormatSize(%d) = %q, want %q", test.bytes, got, test.want)
		}
	}
}

func TestDeriveDefaultsFromHostFacts(t *testing.T) {
	tests := []struct {
		name          string
		facts         HostFacts
		wantMemory    string
		wantCPU       float64
		wantDisk      string
		wantProcesses int
	}{
		{
			// The operator's box today: one vCPU and 4 GiB. The old compiled
			// default handed a single container the whole machine.
			name:          "1 vCPU / 4GiB host",
			facts:         HostFacts{MemoryBytes: 4 * gib, CPUs: 1, DiskBytes: 40 * gib},
			wantMemory:    "3GiB",
			wantCPU:       1,
			wantDisk:      "10GiB",
			wantProcesses: 2000,
		},
		{
			name:          "4 vCPU / 16GiB host clamps memory at the ceiling",
			facts:         HostFacts{MemoryBytes: 16 * gib, CPUs: 4, DiskBytes: 200 * gib},
			wantMemory:    "4GiB",
			wantCPU:       3,
			wantDisk:      "20GiB",
			wantProcesses: 2000,
		},
		{
			name:          "tiny 1GiB host keeps only what the reserve leaves",
			facts:         HostFacts{MemoryBytes: 1 * gib, CPUs: 1, DiskBytes: 8 * gib},
			wantMemory:    "256MiB",
			wantCPU:       1,
			wantDisk:      "5GiB",
			wantProcesses: 2000,
		},
		{
			name:          "unreadable host facts fall back to the floors",
			facts:         HostFacts{},
			wantMemory:    "1GiB",
			wantCPU:       1,
			wantDisk:      "5GiB",
			wantProcesses: 2000,
		},
		{
			name:          "huge host still clamps to the unattended ceiling",
			facts:         HostFacts{MemoryBytes: 256 * gib, CPUs: 64, DiskBytes: 4 * tib},
			wantMemory:    "4GiB",
			wantCPU:       63,
			wantDisk:      "20GiB",
			wantProcesses: 2000,
		},
		{
			name:          "memory floors onto a 512MiB boundary",
			facts:         HostFacts{MemoryBytes: 3 * gib, CPUs: 2, DiskBytes: 50 * gib},
			wantMemory:    "2GiB",
			wantCPU:       1,
			wantDisk:      "12GiB",
			wantProcesses: 2000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := DeriveDefaults(test.facts, DefaultReserve())
			if got.Memory != test.wantMemory {
				t.Errorf("memory = %q, want %q", got.Memory, test.wantMemory)
			}
			if got.CPU != test.wantCPU {
				t.Errorf("cpu = %g, want %g", got.CPU, test.wantCPU)
			}
			if got.Disk != test.wantDisk {
				t.Errorf("disk = %q, want %q", got.Disk, test.wantDisk)
			}
			if got.Processes != test.wantProcesses {
				t.Errorf("processes = %d, want %d", got.Processes, test.wantProcesses)
			}
		})
	}
}

func TestDeriveSettingsProducesAValidDocument(t *testing.T) {
	facts := []HostFacts{
		{MemoryBytes: 4 * gib, CPUs: 1, DiskBytes: 40 * gib},
		{MemoryBytes: 16 * gib, CPUs: 8, DiskBytes: 500 * gib},
		{MemoryBytes: 1 * gib, CPUs: 1, DiskBytes: 8 * gib},
		{},
	}
	for _, host := range facts {
		settings := DeriveSettings(host)
		if !settings.Derived {
			t.Fatalf("derived settings must be marked derived: %+v", settings)
		}
		if err := Validate(settings, host); err != nil {
			t.Fatalf("derived settings for %+v failed validation: %v", host, err)
		}
	}
}
