package weather

import "strings"

type ResolvedCity struct {
	Name   string
	Adcode string
}

type Resolver struct{}

func NewResolver() *Resolver {
	return &Resolver{}
}

func (r *Resolver) Resolve(input string) (ResolvedCity, bool) {
	_ = r

	name := strings.TrimSpace(input)
	if name == "" {
		return ResolvedCity{}, false
	}

	if adcode, ok := adcodeByName[name]; ok {
		return ResolvedCity{Name: name, Adcode: adcode}, true
	}
	if isAdcode(name) {
		return ResolvedCity{Name: name, Adcode: name}, true
	}

	for _, suffix := range lookupSuffixes {
		candidate := name + suffix
		if adcode, ok := adcodeByName[candidate]; ok {
			return ResolvedCity{Name: candidate, Adcode: adcode}, true
		}
	}

	return ResolvedCity{}, false
}

var lookupSuffixes = []string{
	"市",
	"区",
	"县",
	"省",
	"新区",
	"特别行政区",
	"自治区",
	"自治州",
	"自治县",
	"地区",
	"盟",
	"旗",
}

func isAdcode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
