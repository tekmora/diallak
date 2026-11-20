package serverdef

// Invite describes a durable invite to a channel.
type Invite struct {
	// ID identifies the invite in the definition.
	ID string `json:"id"`
	// Channel names the definition channel.
	Channel string `json:"channel"`
	// Purpose describes the invite’s use.
	Purpose string `json:"purpose"`
	// MaxAge is the lifetime in seconds; zero means no expiry.
	MaxAge int `json:"max_age"`
	// MaxUses is the use limit; zero means unlimited.
	MaxUses int `json:"max_uses"`
	// Temporary reports whether membership is temporary.
	Temporary bool `json:"temporary"`
	// Code records the published invite code, if known.
	Code string `json:"code,omitempty"`
}
