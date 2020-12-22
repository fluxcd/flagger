package notifier

type Interface interface {
	Post(workload string, namespace string, message string, fields []Field, severity string) error
}

type Field struct {
	Name  string
	Value string
}
