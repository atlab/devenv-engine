package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//
// -------------------- CPU --------------------
//

func Test_normalizeToCPUText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want string
		ok   bool // true means expect no error
	}{
		// Absent / empty
		{"nil -> empty", nil, "", true},
		{"empty string -> empty", "   ", "", true},

		// Already millicores (lowercased, trimmed, leading zeros collapsed)
		{"millicores simple", "500m", "500m", true},
		{"millicores uppercase M", "500M", "500m", true}, // lowercased
		{"millicores trimmed", "  0500m  ", "500m", true},
		{"millicores zero", "000m", "0m", true},
		{"millicores negative -> error", "-100m", "", false},

		// Core-based numbers in text (keep minimal decimals)
		{"int string", "2", "2", true},
		{"float string", "2.5", "2.5", true},
		{"float trimmed", "  3  ", "3", true},
		{"invalid string", "abc", "", false},

		// Numeric types
		{"int", 4, "4", true},
		{"int64", int(7), "7", true},
		{"float64", 1.25, "1.25", true},
		{"float64 NaN -> error", math.NaN(), "", false},
		{"float64 +Inf -> error", math.Inf(+1), "", false},

		// Unsupported type
		{"bool -> error", true, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeToCPUText(tc.in)
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_cpuTextToMillicores(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int64
		ok   bool // true means expect no error
	}{
		// Empty means "not specified"
		{"empty -> 0,nil", "", 0, true},

		// Millicores
		{"500m -> 500", "500m", 500, true},
		{"0m -> 0", "0m", 0, true},
		{"invalid millicores", "abc m", 0, false},
		{"negative millicores -> error", "-1m", 0, false},

		// Core numbers
		{"2 -> 2000", "2", 2000, true},
		{"2.5 -> 2500", "2.5", 2500, true},
		{"negative cores -> error", "-1", 0, false},
		{"nonnumeric -> error", "abc", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cpuTextToMillicores(tc.in)
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_getCanonicalCPU_Integration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  any
		want int64
		ok   bool // true means expect no error
	}{
		{"nil -> 0", nil, 0, true},
		{"'500m' -> 500", "500m", 500, true},
		{"'2.5' -> 2500", "2.5", 2500, true},
		{"int 3 -> 3000", 3, 3000, true},
		{"invalid -> error", "abc", 0, false},
		{"negative string -> error", "-1", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &ResourceConfig{CPU: tc.raw}
			got, err := r.getCanonicalCPU()
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

//
// -------------------- Memory --------------------
//

func Test_normalizeToMemoryText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want string
		ok   bool // true means expect no error
	}{
		// Absent / empty
		{"nil -> empty", nil, "", true},
		{"empty -> empty", "   ", "", true},

		// Binary SI (case-insensitive) → canonical case preserved
		{"'512mi' -> '512Mi'", "512mi", "512Mi", true},
		{"' 2gi ' -> '2Gi'", " 2gi ", "2Gi", true},
		{"'1Ti' -> '1Ti'", "1Ti", "1Ti", true},

		// Decimal bytes (keep suffix case)
		{"'500M' -> '500M'", "500M", "500M", true},
		{"'1G' -> '1G'", "1G", "1G", true},

		// Bare numeric
		{"'1.5' -> '1.5'", "1.5", "1.5", true},

		// Numeric types
		{"int -> '2'", 2, "2", true},
		{"float64 -> '1.25'", 1.25, "1.25", true},
		{"float NaN -> error", math.NaN(), "", false},
		{"float Inf -> error", math.Inf(+1), "", false},

		// Unsupported type
		{"bool -> error", true, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeToMemoryText(tc.in)
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_memoryTextToMi(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int64
		ok   bool // true means expect no error
	}{
		{"empty -> 0,nil", "", 0, true},

		// Binary SI
		{"16Gi -> 16384", "16Gi", 16 * 1024, true},
		{"512Mi -> 512", "512Mi", 512, true},
		{"1024Ki -> 1", "1024Ki", 1, true},
		{"1.5Gi -> 1536", "1.5Gi", 1536, true},

		// Decimal bytes → Mi (rounded)
		{"500M -> 477Mi", "500M", 477, true},
		{"1G -> 954Mi", "1G", 954, true},

		// Bare number => Gi policy
		{"'15' -> 15360", "15", 15 * 1024, true},
		{"'1.25' -> 1280", "1.25", 1280, true},

		// Rejections / errors
		{"negative Gi -> error", "-1Gi", 0, false},
		{"invalid unit -> error", "12GB", 0, false},
		{"nonnumeric -> error", "abc", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := memoryTextToMi(tc.in)
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_getCanonicalMemory_Integration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  any
		want int64
		ok   bool
	}{
		{"nil -> 0", nil, 0, true},
		{"Gi", "2Gi", 2 * 1024, true},
		{"Mi", "512Mi", 512, true},
		{"decimal bytes", "500M", 477, true},
		{"bare Gi", "1.5", 1536, true},
		{"int Gi", 2, 2 * 1024, true},
		{"float Gi", 1.25, 1280, true},
		{"invalid", "abc", 0, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := &ResourceConfig{Memory: tc.raw}
			got, err := r.getCanonicalMemory()
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

//
// -------------------- Helpers (spot checks) --------------------
//

func Test_bytesToMi_SpotChecks(t *testing.T) {
	t.Parallel()

	// 1 byte -> ~9.5e-7 Mi -> rounds to 0 without error
	got, err := bytesToMi(1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got)

	// Negative -> error
	_, err = bytesToMi(-1)
	require.Error(t, err)

	// Very large -> overflow error (simulate near MaxInt64 Mi in bytes)
	_, err = bytesToMi(math.MaxFloat64)
	require.Error(t, err)
}

func Test_roundFloatToInt64_SpotChecks(t *testing.T) {
	t.Parallel()

	// Normal rounding
	got, err := roundFloatToInt64(1.49)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)

	got, err = roundFloatToInt64(1.5)
	require.NoError(t, err)
	assert.Equal(t, int64(2), got)

	// Tiny non-zero -> 0, no error
	got, err = roundFloatToInt64(1e-12)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got)

	// NaN / Inf -> error
	_, err = roundFloatToInt64(math.NaN())
	require.Error(t, err)
	_, err = roundFloatToInt64(math.Inf(+1))
	require.Error(t, err)
}
