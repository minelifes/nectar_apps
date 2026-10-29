// Package update checks for new versions of the panel, nectar-gateway and k3s, plans compatible
// updates and runs them through a short-lived updater Job (PLAN.md §5a).
package update

import (
	"strconv"
	"strings"
)

// Semver is a parsed semantic version (build metadata is ignored).
type Semver struct {
	Major, Minor, Patch int
	Pre                 string // "" for releases, e.g. "beta.2"
}

// ParseSemver accepts "1.2.3", "v1.2.3", "1.2.3-beta.1" and "nectar-gateway-v1.2.3".
func ParseSemver(s string) (Semver, bool) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "-v"); i >= 0 && strings.IndexByte(s[:i+1], '.') < 0 {
		s = s[i+1:]
	}
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v Semver
	core := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core, v.Pre = s[:i], s[i+1:]
		if v.Pre == "" {
			return Semver{}, false
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Semver{}, false
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" {
			return Semver{}, false
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, true
}

func (v Semver) String() string {
	s := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare returns -1, 0 or 1. Pre-releases sort before their release (1.2.0-beta.1 < 1.2.0).
func (v Semver) Compare(o Semver) int {
	for _, d := range [3]int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	return comparePre(v.Pre, o.Pre)
}

func comparePre(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		case ea == nil:
			return -1
		case eb == nil:
			return 1
		default:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(pa) < len(pb):
		return -1
	case len(pa) > len(pb):
		return 1
	}
	return 0
}

// Less reports v < o.
func (v Semver) Less(o Semver) bool { return v.Compare(o) < 0 }

// Bump classifies the step from v to o: "major", "minor", "patch" or "" when o is not newer.
func (v Semver) Bump(o Semver) string {
	switch {
	case o.Compare(v) <= 0:
		return ""
	case o.Major != v.Major:
		return "major"
	case o.Minor != v.Minor:
		return "minor"
	}
	return "patch"
}

// Satisfies checks a constraint list like ">=0.6.0", ">=1.2.0 <2.0.0", "^1.2", "1.4.x" or "" (anything).
// Several conditions separated by spaces or commas must all hold.
func (v Semver) Satisfies(constraint string) bool {
	for _, c := range strings.FieldsFunc(constraint, func(r rune) bool { return r == ' ' || r == ',' }) {
		if !v.satisfiesOne(c) {
			return false
		}
	}
	return true
}

func (v Semver) satisfiesOne(c string) bool {
	op := ""
	for _, p := range []string{">=", "<=", "!=", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(c, p) {
			op, c = p, c[len(p):]
			break
		}
	}
	// wildcards and short forms: 1.4.x, 1.4, 1
	parts := strings.Split(strings.TrimPrefix(c, "v"), ".")
	for len(parts) < 3 {
		parts = append(parts, "x")
	}
	fixed := 3 // number of leading fields that are not wildcards
	for i, p := range parts {
		if p == "x" || p == "*" || p == "X" {
			fixed = i
			for j := i; j < 3; j++ {
				parts[j] = "0"
			}
			break
		}
	}
	base, ok := ParseSemver(strings.Join(parts, "."))
	if !ok {
		return false
	}
	if fixed < 3 && (op == "" || op == "=") {
		got, want := [3]int{v.Major, v.Minor, v.Patch}, [3]int{base.Major, base.Minor, base.Patch}
		for i := 0; i < fixed; i++ {
			if got[i] != want[i] {
				return false
			}
		}
		return v.Pre == ""
	}
	cmp := v.Compare(base)
	switch op {
	case ">=":
		return cmp >= 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case "<":
		return cmp < 0
	case "!=":
		return cmp != 0
	case "^": // same major (same minor for 0.x)
		if cmp < 0 {
			return false
		}
		if base.Major == 0 {
			return v.Major == 0 && v.Minor == base.Minor
		}
		return v.Major == base.Major
	case "~": // same major.minor
		return cmp >= 0 && v.Major == base.Major && v.Minor == base.Minor
	}
	return cmp == 0
}
