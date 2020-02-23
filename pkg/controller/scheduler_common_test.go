package controller

func alwaysReady() bool {
	return true
}

func toFloatPtr(val int) *float64 {
	v := float64(val)
	return &v
}
