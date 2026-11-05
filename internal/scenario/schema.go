package scenario

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ConflictPolicy specifies how concurrent edits on both sides are expected to resolve.
type ConflictPolicy string

const (
	ConflictPolicyDefault      ConflictPolicy = "default"
	ConflictPolicyLWW          ConflictPolicy = "lww"
	ConflictPolicyConflictCopy ConflictPolicy = "conflict_copy"
)

// SizeRange represents a minimum and maximum byte size for generated files.
type SizeRange struct {
	MinBytes int64
	MaxBytes int64
}

// JitterSpec defines the seeded jitter window in milliseconds after an op_index trigger.
type JitterSpec struct {
	MinMS  int
	MaxMS  int
	Seeded bool
}

// FaultTrigger specifies when a fault injector fires.
type FaultTrigger struct {
	OpIndex  int
	JitterMS JitterSpec
}

// FaultSpec defines the fault injected during a scenario run.
type FaultSpec struct {
	Type   string
	At     FaultTrigger
	Params map[string]string
}

// WorkloadSpec defines the workload recipe and parameters.
type WorkloadSpec struct {
	Recipe   string
	Depth    int
	Files    int
	OpsCount int
	FileSize SizeRange
	Sides    []string
	Params   map[string]string
}

// ExpectSpec defines scenario-level oracle overrides.
type ExpectSpec struct {
	ConflictPolicy ConflictPolicy
}

// RunsSpec defines the seed list and wall-clock timeout per run.
type RunsSpec struct {
	Seeds    []uint64
	TimeoutS int
}

// Scenario represents a loaded and validated SyncFault YAML scenario.
type Scenario struct {
	Name        string
	Description string
	Workload    WorkloadSpec
	Fault       FaultSpec
	Expect      ExpectSpec
	Runs        RunsSpec
}

// LoadFile reads and parses a YAML scenario file from disk.
func LoadFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario %s: %w", path, err)
	}
	return Parse(string(data))
}

// Parse parses the normative SyncFault YAML scenario DSL.
func Parse(raw string) (*Scenario, error) {
	sc := &Scenario{
		Workload: WorkloadSpec{
			Depth:    3,
			Files:    16,
			OpsCount: 20,
			FileSize: SizeRange{MinBytes: 1024, MaxBytes: 65536},
			Sides:    []string{"A"},
			Params:   make(map[string]string),
		},
		Fault: FaultSpec{
			Type:   "none",
			Params: make(map[string]string),
		},
		Expect: ExpectSpec{
			ConflictPolicy: ConflictPolicyDefault,
		},
		Runs: RunsSpec{
			Seeds:    []uint64{1},
			TimeoutS: 180,
		},
	}

	var section, subsection string
	lines := strings.Split(raw, "\n")
	for lineNo, line := range lines {
		clean := stripYAMLComment(line)
		if strings.TrimSpace(clean) == "" {
			continue
		}
		indent := countLeadingSpaces(clean)
		trimmed := strings.TrimSpace(clean)

		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key: value syntax in %q", lineNo+1, trimmed)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch indent {
		case 0:
			section = key
			subsection = ""
			switch key {
			case "name":
				sc.Name = unquote(val)
			case "description":
				sc.Description = unquote(val)
			case "workload", "fault", "expect", "runs":
			default:
				return nil, fmt.Errorf("line %d: unknown top-level key %q", lineNo+1, key)
			}
		case 2:
			subsection = key
			if err := applyLevel2(sc, section, key, val); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
			}
		default:
			if indent >= 4 {
				if err := applyLevel3(sc, section, subsection, key, val); err != nil {
					return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
				}
			}
		}
	}

	if sc.Name == "" {
		return nil, fmt.Errorf("scenario missing required field 'name'")
	}
	if sc.Workload.Recipe == "" {
		return nil, fmt.Errorf("scenario %q missing required field 'workload.recipe'", sc.Name)
	}
	return sc, nil
}

func applyLevel2(sc *Scenario, section, key, val string) error {
	switch section {
	case "workload":
		switch key {
		case "recipe":
			sc.Workload.Recipe = unquote(val)
		case "params":
		case "sides":
			sides, err := parseStringList(val)
			if err != nil {
				return err
			}
			sc.Workload.Sides = sides
		default:
			return fmt.Errorf("unknown workload key %q", key)
		}
	case "fault":
		switch key {
		case "type":
			sc.Fault.Type = unquote(val)
		case "at":
		case "params":
			if val != "" && val != "{}" {
				m, err := parseInlineMap(val)
				if err != nil {
					return err
				}
				for k, v := range m {
					sc.Fault.Params[k] = v
				}
			}
		default:
			return fmt.Errorf("unknown fault key %q", key)
		}
	case "expect":
		switch key {
		case "conflict_policy":
			cp := ConflictPolicy(unquote(val))
			switch cp {
			case ConflictPolicyDefault, ConflictPolicyLWW, ConflictPolicyConflictCopy:
				sc.Expect.ConflictPolicy = cp
			default:
				return fmt.Errorf("invalid conflict_policy %q", val)
			}
		default:
			return fmt.Errorf("unknown expect key %q", key)
		}
	case "runs":
		switch key {
		case "seeds":
			seeds, err := parseSeedsSpec(val)
			if err != nil {
				return err
			}
			sc.Runs.Seeds = seeds
		case "timeout_s":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid timeout_s %q", val)
			}
			sc.Runs.TimeoutS = n
		default:
			return fmt.Errorf("unknown runs key %q", key)
		}
	}
	return nil
}

func applyLevel3(sc *Scenario, section, subsection, key, val string) error {
	switch section {
	case "workload":
		if subsection == "params" {
			sc.Workload.Params[key] = unquote(val)
			switch key {
			case "depth":
				n, err := strconv.Atoi(val)
				if err != nil {
					return err
				}
				sc.Workload.Depth = n
			case "files":
				n, err := strconv.Atoi(val)
				if err != nil {
					return err
				}
				sc.Workload.Files = n
			case "ops":
				n, err := strconv.Atoi(val)
				if err != nil {
					return err
				}
				sc.Workload.OpsCount = n
			case "file_size":
				m, err := parseInlineMap(val)
				if err != nil {
					return err
				}
				if minStr, ok := m["min"]; ok {
					b, err := ParseByteSize(minStr)
					if err != nil {
						return err
					}
					sc.Workload.FileSize.MinBytes = b
				}
				if maxStr, ok := m["max"]; ok {
					b, err := ParseByteSize(maxStr)
					if err != nil {
						return err
					}
					sc.Workload.FileSize.MaxBytes = b
				}
			}
		}
	case "fault":
		if subsection == "at" {
			switch key {
			case "op_index":
				n, err := strconv.Atoi(val)
				if err != nil {
					return err
				}
				sc.Fault.At.OpIndex = n
			case "jitter_ms":
				m, err := parseInlineMap(val)
				if err != nil {
					return err
				}
				if v, ok := m["min"]; ok {
					n, err := strconv.Atoi(v)
					if err != nil {
						return err
					}
					sc.Fault.At.JitterMS.MinMS = n
				}
				if v, ok := m["max"]; ok {
					n, err := strconv.Atoi(v)
					if err != nil {
						return err
					}
					sc.Fault.At.JitterMS.MaxMS = n
				}
				if v, ok := m["seeded"]; ok {
					sc.Fault.At.JitterMS.Seeded = v == "true"
				}
			}
		} else if subsection == "params" {
			sc.Fault.Params[key] = unquote(val)
		}
	}
	return nil
}

// ParseByteSize parses human-friendly byte strings such as "512B", "4KiB", "2MiB", or plain integers.
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		mult   int64
	}{
		{"GiB", 1024 * 1024 * 1024},
		{"MiB", 1024 * 1024},
		{"KiB", 1024},
		{"GB", 1000 * 1000 * 1000},
		{"MB", 1000 * 1000},
		{"KB", 1000},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			numPart := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			n, err := strconv.ParseInt(numPart, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
			}
			return n * u.mult, nil
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

func parseInlineMap(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return nil, fmt.Errorf("expected inline map {...}, got %q", s)
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	out := make(map[string]string)
	if inner == "" {
		return out, nil
	}
	for _, part := range strings.Split(inner, ",") {
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("invalid inline map entry %q", part)
		}
		out[strings.TrimSpace(k)] = unquote(strings.TrimSpace(v))
	}
	return out, nil
}

func parseStringList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected list [...], got %q", s)
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return nil, nil
	}
	var out []string
	for _, item := range strings.Split(inner, ",") {
		out = append(out, unquote(strings.TrimSpace(item)))
	}
	return out, nil
}

func parseSeedsSpec(s string) ([]uint64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected seeds list [...], got %q", s)
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if strings.Contains(inner, "..") {
		startStr, endStr, _ := strings.Cut(inner, "..")
		start, err1 := strconv.ParseUint(strings.TrimSpace(startStr), 10, 64)
		end, err2 := strconv.ParseUint(strings.TrimSpace(endStr), 10, 64)
		if err1 != nil || err2 != nil || end < start {
			return nil, fmt.Errorf("invalid seed range %q", s)
		}
		out := make([]uint64, 0, end-start+1)
		for i := start; i <= end; i++ {
			out = append(out, i)
		}
		return out, nil
	}
	var out []uint64
	for _, part := range strings.Split(inner, ",") {
		v, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid seed %q: %w", part, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func stripYAMLComment(line string) string {
	inQuotes := false
	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			inQuotes = !inQuotes
		} else if line[i] == '#' && !inQuotes {
			return line[:i]
		}
	}
	return line
}

func countLeadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}
