package domain

//----------------------------------------------------------------------------------------------------
// Structs for Outgoing Payloads (NATS -> Signal CLI RPC)
//----------------------------------------------------------------------------------------------------

// SignalOutboundMessage represents the JSON payload received from the NATS 'outbound' topic.
type SignalOutboundMessage struct {
	Recipient  string `json:"recipient"`
	Message    string `json:"message"`
	Attachment string `json:"attachment,omitempty"`
	Account    string `json:"account"`
}

// SignalRPCRequest represents the full JSON-RPC payload to be sent to signal-cli's API.
type SignalRPCRequest struct {
	JSONRPC string                `json:"jsonrpc"`
	Method  string                `json:"method"`
	Params  SignalOutboundMessage `json:"params"`
	ID      int                   `json:"id"`
}

// OutboundNatsMessagePayload represents the payload structure for messages received from NATS 'outbound' topic.
// It wraps the SignalOutboundMessage with a unique ID and server-side timestamp for internal tracking.
type OutboundNatsMessagePayload struct {
	ID              string                 `json:"id"`
	Server          string                 `json:"server"`
	TimestampServer int64                  `json:"timestamp"`
	Message         *SignalOutboundMessage `json:"message"`
}
