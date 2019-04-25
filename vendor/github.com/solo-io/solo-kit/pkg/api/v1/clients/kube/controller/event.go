package controller

type eventType string

const (
	added   eventType = "added"
	updated eventType = "updated"
	deleted eventType = "deleted"
)

type event struct {
	new interface{}
	// only populated for updates
	old       interface{}
	eventType eventType
}
