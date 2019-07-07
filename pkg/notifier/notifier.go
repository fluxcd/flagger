package notifier

type Interface interface {
	Post(workload string, namespace string, message string, fields []Field, warn bool) error
}

type Field struct {
	Name  string
	Value string
}
