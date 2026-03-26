package app

type App struct {
	ID                     string
	Key                    string
	Secret                 string
	WebhookURL             string
	ClientEvents           bool
	MaxConnections         int
	MaxClientEventsPerSec  int
	MaxBackendEventsPerSec int
}
