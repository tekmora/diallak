package serverdef

// Invite describes a durable invite to a channel.
type Invite struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Purpose   string `json:"purpose"`
	MaxAge    int    `json:"max_age"`
	MaxUses   int    `json:"max_uses"`
	Temporary bool   `json:"temporary"`
	Code      string `json:"code,omitempty"`
}
