package proto

import "encoding/json"

type Event struct {
	Event   string `json:"event"`
	Channel string `json:"channel,omitempty"`
	Data    string `json:"data"`
	UserID  string `json:"user_id,omitempty"`
}

// UnmarshalJSON handles the Pusher protocol where `data` can be either
// a JSON string (from server-side SDKs) or a JSON object (from pusher-js).
func (e *Event) UnmarshalJSON(b []byte) error {
	type Alias struct {
		Event   string          `json:"event"`
		Channel string          `json:"channel,omitempty"`
		Data    json.RawMessage `json:"data"`
		UserID  string          `json:"user_id,omitempty"`
	}
	var a Alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	e.Event = a.Event
	e.Channel = a.Channel
	e.UserID = a.UserID

	if len(a.Data) == 0 {
		e.Data = ""
	} else if a.Data[0] == '"' {
		// It's a JSON string — unwrap the quotes
		var s string
		if err := json.Unmarshal(a.Data, &s); err != nil {
			return err
		}
		e.Data = s
	} else {
		// It's an object/array — keep as raw JSON string
		e.Data = string(a.Data)
	}
	return nil
}

type SubscribeData struct {
	Channel     string `json:"channel"`
	Auth        string `json:"auth"`
	ChannelData string `json:"channel_data"`
}

func NewEvent(event, channel string, data any) (Event, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	return Event{
		Event:   event,
		Channel: channel,
		Data:    string(encoded),
	}, nil
}

func NewRawEvent(event, channel, data string) Event {
	return Event{
		Event:   event,
		Channel: channel,
		Data:    data,
	}
}

func ParseSubscribeData(data string) (SubscribeData, error) {
	var sd SubscribeData
	err := json.Unmarshal([]byte(data), &sd)
	return sd, err
}

// Subscriber is implemented by ws.Conn to break the import cycle between ws and channel.
type Subscriber interface {
	SocketID() string
	SendEvent(event Event) error
	AddChannel(name string)
	RemoveChannel(name string)
}
