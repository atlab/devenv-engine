package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ============================================================================
// --- CPU normalization pipeline ---------------------------------------------
// ============================================================================

// normalizeToCPUText coerces a flexible value (string/int/float) into a
// canonical textual CPU quantity that k8s accepts, e.g. "2", "2.5", or "500m".
// It trims whitespace and lowercases the unit. It does NOT add "m" for you;
// integers/floats remain core-based unless the input already used "m".
func normalizeToCPUText(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil // absent; caller decides how to treat
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return "", nil
		}
		s = strings.ToLower(s)
		// Already millicores, e.g. "500m"
		if strings.HasSuffix(s, "m") {
			d := strings.TrimSpace(strings.TrimSuffix(s, "m"))
			// Reject signs explicitly (ParseUint also rejects "-")
			if strings.HasPrefix(d, "+") || strings.HasPrefix(d, "-") {
				return "", fmt.Errorf("invalid millicores: %q", x)
			}
			// digits-only, non-negative, and no overflow
			if _, err := strconv.ParseUint(d, 10, 64); err != nil {
				return "", fmt.Errorf("invalid millicores: %q", x)
			}
			// normalize leading zeros (keep a single "0" if all zeros)
			d = strings.TrimLeft(d, "0")
			if d == "" {
				d = "0"
			}
			return d + "m", nil
		}
		// Otherwise it must be a float/int string like "2", "2.5"
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "", fmt.Errorf("invalid cpu number: %q", s)
		}
		// Keep minimal decimal; "2" stays "2", "2.5" stays "2.5"
		return strconv.FormatFloat(f, 'f', -1, 64), nil

	case int:
		return strconv.FormatInt(int64(x), 10), nil

	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return "", fmt.Errorf("invalid cpu number: %v", x)
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil

	default:
		return "", fmt.Errorf("unsupported cpu type: %T", v)
	}
}

// cpuTextToMillicores converts a textual CPU quantity ("2", "2.5", "500m")
// into millicores.
// Policy:
//   - empty text => 0, nil (treat as "not specified")
//   - negative => error
//   - "Xm" => parse as integer millicores
//   - number => cores → round(f*1000) millicores
func cpuTextToMillicores(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "m") {
		d := strings.TrimSuffix(s, "m")
		n, err := strconv.ParseInt(d, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid millicores: %q", s)
		}
		if n < 0 {
			return 0, fmt.Errorf("cpu must be >= 0 millicores (got %d)", n)
		}
		return n, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cpu number: %q", s)
	}
	if f < 0 {
		return 0, fmt.Errorf("cpu must be >= 0 cores (got %v)", f)
	}
	return int64(math.Round(f * 1000.0)), nil
}

// getCanonicalCPU parses ResourceConfig.CPU on demand and returns millicores.
// This is the single entry-point your higher-level code should call.
func (r *ResourceConfig) getCanonicalCPU() (int64, error) {
	text, err := normalizeToCPUText(r.CPU)
	if err != nil {
		return 0, err
	}
	return cpuTextToMillicores(text)
}

// ============================================================================
// --- memory normalization pipeline ------------------------------------------
// ============================================================================

// normalizeMemoryText coerces a flexible raw value (string/int/float/…) into a
// normalized textual quantity. It preserves the user's units (if supplied) but
// canonicalizes unit casing (e.g., "mi" -> "Mi", "g" -> "G") and trims space.
//
// Examples in -> out:
//
//	" 16Gi "   -> "16Gi"
//	"512mi"    -> "512Mi"
//	"500m"     -> "500m"
//	"1.5"      -> "1.5"
//	2          -> "2"
//	1.25       -> "1.25"
func normalizeToMemoryText(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil

	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return "", nil
		}
		Misuffixes := [6]string{"Ki", "Mi", "Gi", "Ti", "Pi", "Ei"}
		for _, suffix := range Misuffixes {
			if hasSuffixFold(s, suffix) {
				return strings.TrimSpace(s[:len(s)-2]) + suffix, nil
			}
		}

		Msuffixes := [6]string{"K", "M", "G", "T", "P", "E"}
		for _, suffix := range Msuffixes {
			if hasSuffixFold(s, suffix) {
				return strings.TrimSpace(s[:len(s)-1]) + suffix, nil
			}
		}
		return s, nil

	case int:
		return strconv.FormatInt(int64(x), 10), nil

	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return "", fmt.Errorf("invalid memory number: %v", x)
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil

	default:
		return "", fmt.Errorf("unsupported memory type: %T", v)
	}
}

var binToMi = map[string]float64{
	"Ki": 1.0 / 1024.0,
	"Mi": 1.0,
	"Gi": 1024.0,
	"Ti": 1024.0 * 1024.0,
	"Pi": 1024.0 * 1024.0 * 1024.0,
	"Ei": 1024.0 * 1024.0 * 1024.0 * 1024.0,
}

var decBytesToMi = map[string]float64{
	"k": 1e3,
	"M": 1e6,
	"G": 1e9,
	"T": 1e12,
	"P": 1e15,
	"E": 1e18,
}

// memoryTextToMi converts a normalized textual quantity to canonical MiB.
// Policy:
//   - empty => (0, nil)  // “not specified”
//   - negative => error
//   - binary units: Ki/Mi/Gi/Ti/Pi/Ei
//   - decimal bytes: k/M/G/T/P/E (10^3 … 10^18 bytes), converted to Mi
//   - bare number => Gi by policy (e.g., "1.5" Gi -> 1536 Mi)
//   - rejects CPU-like "m" suffix
func memoryTextToMi(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	// Try binary suffixes (Ki/Mi/Gi/…)
	for suf, factor := range binToMi {
		if strings.HasSuffix(strings.ToLower(s), strings.ToLower(suf)) {
			numStr := strings.TrimSpace(s[:len(s)-len(suf)])
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				return 0, fmt.Errorf("invalid %s quantity: %q", suf, s)
			}
			return roundFloatToInt64(n * factor)
		}
	}

	// // Reject CPU-like suffix "m" TODO: Revisit if this is feature to support
	// if strings.HasSuffix(s, "m") || strings.HasSuffix(strings.ToLower(s), "m") {
	// 	return 0, fmt.Errorf("invalid memory unit %q (did you mean CPU millicores?)", s)
	// }

	// Try decimal byte suffixes (k/M/G/…)
	for suf, mul := range decBytesToMi {
		if strings.HasSuffix(strings.ToLower(s), strings.ToLower(suf)) {
			numStr := strings.TrimSpace(s[:len(s)-len(suf)])
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				return 0, fmt.Errorf("invalid %s bytes quantity: %q", suf, s)
			}
			// Convert decimal bytes → MiB.
			bytes := n * mul
			return bytesToMi(bytes)
		}
	}

	// Bare number => Gi by policy
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
		if n < 0 {
			return 0, fmt.Errorf("memory must be >= 0")
		}
		// Bare number => Gi by policy, convert to Mi.
		return roundFloatToInt64(n * 1024.0)
	}
	return 0, fmt.Errorf("invalid memory quantity: %q", s)
}

// getCanonicalMemory parses ResourceConfig.Memory on demand and returns MiB.
func (r *ResourceConfig) getCanonicalMemory() (int64, error) {
	text, err := normalizeToMemoryText(r.Memory)
	if err != nil {
		return 0, err
	}
	return memoryTextToMi(text)
}

// ---------------- helpers ----------------
// bytesToMi converts a size in decimal bytes to mebibytes (MiB) and rounds to
// the nearest int64 MiB. It rejects negative, NaN, or infinite inputs and
// returns an error in those cases. If the converted MiB value would exceed
// math.MaxInt64, it returns an overflow error.
func bytesToMi(bytes float64) (int64, error) {
	if bytes < 0 || math.IsNaN(bytes) || math.IsInf(bytes, 0) {
		return 0, fmt.Errorf("memory must be >= 0")
	}
	miFloat := bytes / (1024.0 * 1024.0)
	if miFloat > float64(math.MaxInt64) {
		return 0, fmt.Errorf("memory overflows supported range")
	}
	return roundFloatToInt64(miFloat)
}

// roundFloatToInt64 rounds v to the nearest int64 using standard rounding.
// NaN or infinite values return an error. Extremely small non-zero magnitudes
// that round to 0 (|v| < 0.5) are treated as 0 without error.
func roundFloatToInt64(v float64) (int64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid number")
	}
	rounded := int64(math.Round(v))
	if rounded == 0 && v != 0 {
		// extremely small but non-zero; treat as 0 Mi without error
		return 0, nil
	}
	return rounded, nil
}

// Case-insensitive suffix check.
func hasSuffixFold(s, suf string) bool {
	return len(s) >= len(suf) && strings.EqualFold(s[len(s)-len(suf):], suf)
}
