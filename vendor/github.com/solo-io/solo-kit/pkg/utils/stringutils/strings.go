package stringutils

import (
	"reflect"
	"sort"
)

func ContainsString(s string, slice []string) bool {
	for _, s2 := range slice {
		if s2 == s {
			return true
		}
	}
	return false
}

// Returns true if slice strings contains any of the strings.
func ContainsAny(strings []string, slice []string) bool {
	for _, s := range strings {
		if ContainsString(s, slice) {
			return true
		}
	}
	return false
}

func ContainsMap(maps []map[string]string, item map[string]string) bool {
	for _, m := range maps {
		if reflect.DeepEqual(m, item) {
			return true
		}
	}
	return false
}

func KeysAndValues(m map[string]string) ([]string, []string) {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var values []string
	for _, k := range keys {
		values = append(values, m[k])
	}
	return keys, values
}

func Unique(stringSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range stringSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
