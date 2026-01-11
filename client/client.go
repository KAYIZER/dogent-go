package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/KAYIZER/dogent-go/executor"
	"github.com/gorilla/websocket"
)

type Config struct {
	ServerURL string
	Token     string
	ServerID  string
}

type AgentClient struct {
	config Config
	conn   *websocket.Conn
	done   chan struct{}
}

func NewAgentClient(cfg Config) *AgentClient {
	return &AgentClient{
		config: cfg,
		done:   make(chan struct{}),
	}
}

func (c *AgentClient) Connect() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u, err := url.Parse(c.config.ServerURL)
	if err != nil {
		log.Fatal("Invalid URL:", err)
	}

	for {
		log.Printf("🔌 Connecting to %s...", u.String())

		// Connect
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("❌ Connection failed: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		c.conn = conn
		log.Println("✅ Connected!")

		// Authenticate
		authMsg := map[string]string{
			"token":     c.config.Token,
			"server_id": c.config.ServerID,
		}
		if err := c.conn.WriteJSON(authMsg); err != nil {
			log.Println("Authentication write failed:", err)
			c.conn.Close()
			continue
		}

		// Listen Loop
		c.listen()

		// If listen returns, it means we disconnected. Loop continues to reconnect.
	}
}

func (c *AgentClient) listen() {
	defer c.conn.Close()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			return
		}

		// Handle Message
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("JSON Parse Error: %v", err)
			continue
		}

		c.handleMessage(msg)
	}
}

func (c *AgentClient) handleMessage(msg map[string]interface{}) {
	msgType, ok := msg["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "status":
		log.Printf("ℹ️ Status: %v", msg["content"])
	case "pong":
		// Heartbeat ack, ignore
	case "command":
		cmdContent, _ := msg["content"].(string)
		log.Printf("📢 Received Command: %s", cmdContent)

		// Execute
		output, err := executor.RunCommand(cmdContent)
		if err != nil {
			log.Printf("Execution Error: %v", err)
			output = fmt.Sprintf("Error: %v", err)
		}

		// Send Result
		result := map[string]string{
			"type":    "command_result",
			"content": output,
		}
		c.conn.WriteJSON(result)

	default:
		log.Printf("Unknown message: %v", msg)
	}
}

func (c *AgentClient) SendMessage(payload interface{}) error {
	if c.conn == nil {
		return fmt.Errorf("connection not established")
	}
	return c.conn.WriteJSON(payload)
}
