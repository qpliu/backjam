package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	// Jamulus protocol port
	DefaultPort = 22124

	// Message types
	MsgTypeAudio  = 0
	MsgTypeChat   = 1
	MsgTypeClient = 2
	MsgTypeServer = 3

	// Audio frame settings
	DefaultSampleRate = 48000
	DefaultChannels   = 2
	DefaultFrameSize  = 960 // 20ms at 48kHz
)

// JamulusChatMessage represents a chat message
type JamulusChatMessage struct {
	FromClientID int
	FromName     string
	Text         string
	Timestamp    time.Time
}

// Client represents a Jamulus client connection
type JamulusClient struct {
	conn       net.Conn
	address    string
	connected  bool
	clientID   int
	clientName string
	sampleRate int
	channels   int
	frameSize  int

	// Audio channels
	audioSendChan    chan []byte
	audioReceiveChan chan []byte

	// Chat channels
	chatSendChan    chan *JamulusChatMessage
	chatReceiveChan chan *JamulusChatMessage

	// Synchronization
	mu        sync.RWMutex
	closeChan chan struct{}
	wg        sync.WaitGroup

	// Callbacks
	onChatReceived func(*JamulusChatMessage)
	onDisconnect   func()
}

// NewClient creates a new Jamulus client
func NewClient(address string) *JamulusClient {
	return &Client{
		address:          address,
		sampleRate:       DefaultSampleRate,
		channels:         DefaultChannels,
		frameSize:        DefaultFrameSize,
		audioSendChan:    make(chan []byte, 10),
		audioReceiveChan: make(chan []byte, 10),
		chatSendChan:     make(chan *JamulusChatMessage, 10),
		chatReceiveChan:  make(chan *JamulusChatMessage, 10),
		closeChan:        make(chan struct{}),
	}
}

// Connect establishes a connection to the Jamulus server
func (c *JamulusClient) Connect() error {
	conn, err := net.Dial("udp", c.address)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.address, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	// Start send and receive goroutines
	c.wg.Add(2)
	go c.sendWorker()
	go c.receiveWorker()

	return nil
}

// Close closes the connection
func (c *JamulusClient) Close() error {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return nil
	}
	c.connected = false
	c.mu.Unlock()

	close(c.closeChan)
	c.wg.Wait()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SetAudioConfig sets the audio configuration
func (c *JamulusClient) SetAudioConfig(sampleRate, channels, frameSize int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sampleRate = sampleRate
	c.channels = channels
	c.frameSize = frameSize
}

// SetClientInfo sets the client name and ID
func (c *JamulusClient) SetClientInfo(clientID int, clientName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientID = clientID
	c.clientName = clientName
}

// SendOpusFrame sends an Opus-encoded audio frame to the server
func (c *JamulusClient) SendOpusFrame(opusData []byte) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("client not connected")
	}
	c.mu.RUnlock()

	select {
	case c.audioSendChan <- opusData:
		return nil
	case <-c.closeChan:
		return fmt.Errorf("client is closing")
	default:
		return fmt.Errorf("send buffer full")
	}
}

// SendChatMessage sends a chat message to the server
func (c *JamulusClient) SendChatMessage(text string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("client not connected")
	}
	clientID := c.clientID
	clientName := c.clientName
	c.mu.RUnlock()

	msg := &JamulusChatMessage{
		FromClientID: clientID,
		FromName:     clientName,
		Text:         text,
		Timestamp:    time.Now(),
	}

	select {
	case c.chatSendChan <- msg:
		return nil
	case <-c.closeChan:
		return fmt.Errorf("client is closing")
	default:
		return fmt.Errorf("send buffer full")
	}
}

// ReceiveAudioFrame receives an audio frame (non-blocking)
func (c *JamulusClient) ReceiveAudioFrame() ([]byte, error) {
	select {
	case frame := <-c.audioReceiveChan:
		return frame, nil
	default:
		return nil, fmt.Errorf("no audio frame available")
	}
}

// ReceiveAudioFrameTimeout receives an audio frame with timeout
func (c *JamulusClient) ReceiveAudioFrameTimeout(timeout time.Duration) ([]byte, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case frame := <-c.audioReceiveChan:
		return frame, nil
	case <-timer.C:
		return nil, fmt.Errorf("receive timeout")
	case <-c.closeChan:
		return nil, fmt.Errorf("client is closing")
	}
}

// ReceiveChatMessage receives a chat message (non-blocking)
func (c *JamulusClient) ReceiveChatMessage() (*JamulusChatMessage, error) {
	select {
	case msg := <-c.chatReceiveChan:
		return msg, nil
	default:
		return nil, fmt.Errorf("no chat message available")
	}
}

// ReceiveChatMessageTimeout receives a chat message with timeout
func (c *JamulusClient) ReceiveChatMessageTimeout(timeout time.Duration) (*JamulusChatMessage, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case msg := <-c.chatReceiveChan:
		return msg, nil
	case <-timer.C:
		return nil, fmt.Errorf("receive timeout")
	case <-c.closeChan:
		return nil, fmt.Errorf("client is closing")
	}
}

// SetOnChatReceived sets a callback for received chat messages
func (c *JamulusClient) SetOnChatReceived(callback func(*JamulusChatMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChatReceived = callback
}

// SetOnDisconnect sets a callback for disconnection events
func (c *JamulusClient) SetOnDisconnect(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDisconnect = callback
}

// sendWorker handles sending audio and chat messages
func (c *JamulusClient) sendWorker() {
	defer c.wg.Done()

	for {
		select {
		case opusData := <-c.audioSendChan:
			if err := c.sendAudioPacket(opusData); err != nil {
				fmt.Printf("Error sending audio: %v\n", err)
			}

		case chatMsg := <-c.chatSendChan:
			if err := c.sendChatPacket(chatMsg); err != nil {
				fmt.Printf("Error sending chat: %v\n", err)
			}

		case <-c.closeChan:
			return
		}
	}
}

// receiveWorker handles receiving audio and chat messages
func (c *JamulusClient) receiveWorker() {
	defer c.wg.Done()

	buffer := make([]byte, 4096)

	for {
		select {
		case <-c.closeChan:
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		// Set read deadline to allow checking closeChan
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := conn.Read(buffer)

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			fmt.Printf("Connection error: %v\n", err)
			c.handleDisconnect()
			return
		}

		if n < 1 {
			continue
		}

		// Parse packet
		msgType := buffer[0]
		payload := buffer[1:n]

		switch msgType {
		case MsgTypeAudio:
			select {
			case c.audioReceiveChan <- payload:
			case <-c.closeChan:
				return
			default:
				// Buffer full, drop frame
			}

		case MsgTypeChat:
			msg, err := c.parseChatPacket(payload)
			if err != nil {
				fmt.Printf("Error parsing chat: %v\n", err)
				continue
			}

			select {
			case c.chatReceiveChan <- msg:
			case <-c.closeChan:
				return
			default:
				// Buffer full, drop message
			}

			// Call callback if set
			c.mu.RLock()
			callback := c.onChatReceived
			c.mu.RUnlock()
			if callback != nil {
				callback(msg)
			}
		}
	}
}

// sendAudioPacket sends an audio packet to the server
func (c *JamulusClient) sendAudioPacket(opusData []byte) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	// Packet format: [msgType (1 byte)][length (2 bytes)][opus data]
	packet := make([]byte, 3+len(opusData))
	packet[0] = MsgTypeAudio
	binary.BigEndian.PutUint16(packet[1:3], uint16(len(opusData)))
	copy(packet[3:], opusData)

	_, err := conn.Write(packet)
	return err
}

// sendChatPacket sends a chat packet to the server
func (c *JamulusClient) sendChatPacket(msg *JamulusChatMessage) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	// Packet format: [msgType (1 byte)][clientID (4 bytes)][nameLen (2 bytes)][name][textLen (2 bytes)][text]
	nameBytes := []byte(msg.FromName)
	textBytes := []byte(msg.Text)

	payloadLen := 4 + 2 + len(nameBytes) + 2 + len(textBytes)
	packet := make([]byte, 1+payloadLen)

	packet[0] = MsgTypeChat
	binary.BigEndian.PutUint32(packet[1:5], uint32(msg.FromClientID))
	binary.BigEndian.PutUint16(packet[5:7], uint16(len(nameBytes)))
	copy(packet[7:], nameBytes)

	offset := 7 + len(nameBytes)
	binary.BigEndian.PutUint16(packet[offset:offset+2], uint16(len(textBytes)))
	copy(packet[offset+2:], textBytes)

	_, err := conn.Write(packet)
	return err
}

// parseChatPacket parses a chat packet
func (c *JamulusClient) parseChatPacket(payload []byte) (*JamulusChatMessage, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("payload too short")
	}

	clientID := binary.BigEndian.Uint32(payload[0:4])
	nameLen := binary.BigEndian.Uint16(payload[4:6])

	if len(payload) < 6+int(nameLen)+2 {
		return nil, fmt.Errorf("invalid payload length")
	}

	name := string(payload[6 : 6+nameLen])
	offset := 6 + int(nameLen)
	textLen := binary.BigEndian.Uint16(payload[offset : offset+2])

	if len(payload) < offset+2+int(textLen) {
		return nil, fmt.Errorf("invalid text length")
	}

	text := string(payload[offset+2 : offset+2+int(textLen)])

	return &JamulusChatMessage{
		FromClientID: int(clientID),
		FromName:     name,
		Text:         text,
		Timestamp:    time.Now(),
	}, nil
}

// handleDisconnect handles disconnection
func (c *JamulusClient) handleDisconnect() {
	c.mu.Lock()
	c.connected = false
	callback := c.onDisconnect
	c.mu.Unlock()

	if callback != nil {
		callback()
	}
}
