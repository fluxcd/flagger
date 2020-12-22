package notifier

type NopNotifier struct{}

func (n *NopNotifier) Post(string, string, string, []Field, string) error {
	return nil
}
