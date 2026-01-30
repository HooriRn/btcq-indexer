package websockets

import "fmt"

const (
	MessageDisconnect = "Disconnect"
	MessageConnect    = "Connect"
)

// Instruction to subscribe and unsubscribe from WS
type Instruction struct {
	Message string   // Disconnect or Connect
	Assets  []string // valid Assets supported within our pools
}

// Payload what we send to clients to convey price updates
type Payload struct {
	Price string
	Asset string // Asset supported within our pools
}

func (p *Payload) ToString() string {
	return fmt.Sprintf("asset %s, price %s", p.Asset, p.Price)
}
