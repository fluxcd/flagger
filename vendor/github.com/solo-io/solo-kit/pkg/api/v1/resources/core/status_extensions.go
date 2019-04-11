package core

import (
	"fmt"
	"sort"
)

// collapse a status into a status with no children
func (s Status) Flatten() Status {
	if len(s.SubresourceStatuses) == 0 {
		return s
	}
	out := Status{
		State:  s.State,
		Reason: s.Reason,
	}
	orderedMapIterator(s.SubresourceStatuses, func(key string, stat *Status) {
		status := stat.Flatten()
		switch status.State {
		case Status_Rejected:
			out.State = Status_Rejected
			out.Reason += key + fmt.Sprintf("child %v rejected with reason: %v.\n", key, status.Reason)
		case Status_Pending:
			if out.State == Status_Accepted {
				out.State = Status_Pending
			}
			out.Reason += key + " is still pending.\n"
		}
	})
	return out
}

func orderedMapIterator(m map[string]*Status, onKey func(key string, value *Status)) {
	var list []struct {
		key   string
		value *Status
	}
	for k, v := range m {
		list = append(list, struct {
			key   string
			value *Status
		}{
			key:   k,
			value: v,
		})
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].key < list[j].key
	})
	for _, el := range list {
		onKey(el.key, el.value)
	}
}
