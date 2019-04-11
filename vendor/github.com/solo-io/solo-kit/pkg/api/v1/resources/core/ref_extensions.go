package core

import (
	"fmt"
)

func (r ResourceRef) Strings() (string, string) {
	return r.Namespace, r.Name
}

func (r ResourceRef) Key() string {
	return fmt.Sprintf("%v.%v", r.Namespace, r.Name)
}
