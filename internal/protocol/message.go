package protocol

import "encoding/json"

type MessageType string

const (
	TypeRegister      MessageType = "register"
	TypeScreenFrame   MessageType = "screen_frame"
	TypeInput         MessageType = "input"
	TypeFileTransfer  MessageType = "file_transfer"
	TypeFileChunk     MessageType = "file_chunk"
	TypeFileEnd       MessageType = "file_end"
	TypePing          MessageType = "ping"
	TypePong          MessageType = "pong"
	TypeError         MessageType = "error"
)

type Message struct {
	Type    MessageType     `json:"type"`
	AgentID string          `json:"agent_id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type RegisterPayload struct {
	AgentID string `json:"agent_id"`
	Role    string `json:"role"` // "agent" or "viewer"
}

type ScreenFramePayload struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Data   []byte `json:"data"` // JPEG compressed
}

type InputPayload struct {
	Type     string `json:"type"` // "mouse_move", "mouse_click", "key_press", "key_release"
	X        int    `json:"x,omitempty"`
	Y        int    `json:"y,omitempty"`
	Button   int    `json:"button,omitempty"` // 1=left, 2=middle, 3=right
	KeyCode  int    `json:"key_code,omitempty"`
	Key      string `json:"key,omitempty"`
}

type FileTransferPayload struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	FileID   string `json:"file_id"`
}

type FileChunkPayload struct {
	FileID string `json:"file_id"`
	Offset int64  `json:"offset"`
	Data   []byte `json:"data"`
}

type FileEndPayload struct {
	FileID string `json:"file_id"`
	Status string `json:"status"` // "success" or "error"
}
