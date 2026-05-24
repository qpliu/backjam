package jamulus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

// Message IDs for connected client protocol
const (
	MsgTypeIllegal              = 0
	MsgTypeAckn                 = 1
	MsgTypeJitterBufSize        = 10
	MsgTypeReqJitterBufSize     = 11
	MsgTypeChannelGain          = 13
	MsgTypeReqConnClientsList   = 16
	MsgTypeChannelName          = 17
	MsgTypeChatText             = 18
	MsgTypeNetworkTransportProp = 20
	MsgTypeReqNetTransportProps = 21
	MsgTypeReqChannelInfo       = 23
	MsgTypeConnClientsList      = 24
	MsgTypeChannelInfo          = 25
	MsgTypeOpusSupported        = 26
	MsgTypeVersionAndOS         = 29
	MsgTypeChannelPan           = 30
	MsgTypeMuteStateChanged     = 31
	MsgTypeClientID             = 32
	MsgTypeRecorderState        = 33
	MsgTypeReqSplitMessSupport  = 34
	MsgTypeSplitMessSupported   = 35
	MsgTypeRawAudioSupported    = 36
)

// Connection-less message IDs
const (
	MsgTypeCLPingMs             = 1001
	MsgTypeCLPingWithNumClients = 1002
	MsgTypeCLServerFull         = 1003
	MsgTypeCLRegisterServer     = 1004
	MsgTypeCLUnregisterServer   = 1005
	MsgTypeCLServerList         = 1006
	MsgTypeCLReqServerList      = 1007
	MsgTypeCLSendEmptyMessage   = 1008
	MsgTypeCLEmptyMessage       = 1009
	MsgTypeCLDisconnection      = 1010
	MsgTypeCLVersionAndOS       = 1011
	MsgTypeCLReqVersionAndOS    = 1012
	MsgTypeCLConnClientsList    = 1013
	MsgTypeCLReqConnClientsList = 1014
	MsgTypeCLChannelLevelList   = 1015
	MsgTypeCLRegisterServerResp = 1016
	MsgTypeCLRegisterServerEx   = 1017
	MsgTypeCLRedServerList      = 1018
)

// Message header constants
const (
	MessageHeaderLength = 7 // TAG (2), ID (2), cnt (1), length (2)
	CRCLength           = 2
	MessageMinLength    = MessageHeaderLength + CRCLength
	MessageSplitSize    = 550
)

// Audio codec types
const (
	AudioCodecNone   = 0
	AudioCodecCELT   = 1
	AudioCodecOpus   = 2
	AudioCodecOpus64 = 3
)

// Network flags
const (
	NetworkFlagNone      = 0
	NetworkFlagWithCount = 1
)

// CRC-16 constants for Jamulus protocol
const (
	crcPolynomial = 0x1020
	crcBitOutMask = 0x10000
	crcInitial    = 0xFFFFFFFF
)

// Sample rate constant
const SYSTEM_SAMPLE_RATE_HZ = 48000

// Raw audio frame sizes - must match exactly for server detection
const (
	RAW_AUDIO_FRAME_SIZE_SAMPLES = 960 // 20ms at 48kHz (128 samples per frame for OPUS)
)

// ClientInfo represents information about a connected client
type ClientInfo struct {
	ChannelID int
	Name      string
	Gain      float32
	Pan       float32
	IsMuted   bool
	Level     uint16
}

// ConnectedClientsInfo represents the list of connected clients
type ConnectedClientsInfo struct {
	Clients []ClientInfo
}

// ChannelInfo represents channel information
type ChannelInfo struct {
	ChannelID  int
	Name       string
	Country    string
	City       string
	IPAddress  string
	Instrument int
	SkillLevel int
}

// NetworkTransportProps holds network transport properties
type NetworkTransportProps struct {
	BasePacketSize   uint32
	BlockSizeFactor  uint16
	NumAudioChannels uint32
	SampleRate       uint32
	AudioCodecType   uint8
	Flags            uint8
	Reserved         uint16
}

// Client represents a Jamulus client
type Client struct {
	conn       *net.UDPConn
	serverAddr *net.UDPAddr
	connected  bool
	mu         sync.RWMutex
	doneCh     chan struct{}
	wg         sync.WaitGroup

	// Client info
	clientID     int
	clientName   string
	localAddress string
	countryCode  string
	city         string
	instrument   int
	skillLevel   int

	// Audio configuration
	audioCodecType   int
	numAudioChannels int
	frameSizeFactor  int
	basePacketSize   int
	frameSizeSamples int

	// Message state
	messageCounter    uint8
	sequenceNumber    uint8
	useSequenceNumber bool

	// Receive channels
	audioFrameCh  chan []byte
	protocolMsgCh chan []byte

	// Callbacks
	onChatReceived         func(string)
	onConnectedClientsInfo func(*ConnectedClientsInfo)
	onChannelGain          func(int, float32)
	onChannelPan           func(int, float32)
	onMuteStateChanged     func(int, bool)
	onServerFull           func()
	onPingReceived         func(int)
	onClientIDReceived     func(int)
	onVersionAndOSReceived func(byte, string)
	onRawAudioSupported    func()
}

// NewClient creates a new Jamulus client
func NewClient(serverAddr string) (*Client, error) {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid server address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	c := &Client{
		conn:              conn,
		serverAddr:        addr,
		connected:         true,
		doneCh:            make(chan struct{}),
		audioFrameCh:      make(chan []byte, 100),
		protocolMsgCh:     make(chan []byte, 100),
		clientID:          -1,
		audioCodecType:    AudioCodecOpus,
		numAudioChannels:  2,
		frameSizeFactor:   1,
		basePacketSize:    512,
		frameSizeSamples:  128,
		useSequenceNumber: false,
	}

	// Start receive loop
	c.wg.Add(1)
	go c.receiveLoop()

	return c, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	c.sendProtocolMessage(MsgTypeCLDisconnection, []byte{})

	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return nil
	}
	c.connected = false
	c.mu.Unlock()

	// Signal the receive loop to stop
	close(c.doneCh)

	// Wait for the receive loop to finish (without holding the mutex)
	c.wg.Wait()

	return c.conn.Close()
}

// SendOpusFrame sends an Opus-encoded audio frame
//
// JAMULUS AUDIO PACKET STRUCTURE (Opus):
// ======================================
// Audio packets are raw UDP datagrams containing ONLY:
// 1. Opus-encoded audio frame data
// 2. Optional sequence counter byte (if negotiated with server)
//
// PACKET LAYOUT:
// Offset  Size  Field              Description
// ------  ----  -----              -----------
// 0       N     Opus Audio Data    Raw Opus codec bitstream
//
//	Size = iCeltNumCodedBytes
//
// N       1     Sequence Counter   Optional: if bUseSequenceNumber=true
//
//	Wraps at 256
//
// TOTAL PACKET SIZE:
// Without sequence: iCeltNumCodedBytes
// With sequence:    iCeltNumCodedBytes + 1
//
// PARAMETERS:
//
//	opusFrame: Raw Opus-encoded audio data matching negotiated size
//	           If bUseSequenceNumber is enabled, this should include
//	           the sequence number byte appended at the end
//
// RETURNS:
//
//	error: Connection error, or nil if sent successfully
func (c *Client) SendOpusFrame(opusFrame []byte) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	msg := c.buildAudioFrame(opusFrame)
	return c.sendMessageConnected(msg)
}

// SendRawAudioFrame sends raw uncompressed PCM audio to the server
//
// JAMULUS AUDIO PACKET STRUCTURE (Raw Audio):
// ==========================================
// Raw audio packets contain uncompressed int16 PCM samples.
//
// PACKET LAYOUT:
// Offset  Size  Field              Description
// ------  ----  -----              -----------
// 0       N     PCM Audio Data     int16 samples in little-endian format
//
//	Size = frameSamples * numChannels * 2 bytes
//
// N       1     Sequence Counter   Optional: if bUseSequenceNumber=true
//
//	Wraps at 256
//
// TOTAL PACKET SIZE:
// Without sequence: (frameSamples * numChannels * 2) bytes
// With sequence:    (frameSamples * numChannels * 2) + 1 bytes
//
// FRAME PARAMETERS:
// - Sample rate: 48 kHz (fixed)
// - Sample format: signed 16-bit little-endian
// - Frame duration: 20ms = 960 samples per channel
// - Channels: 1 (mono) or 2 (stereo)
//
// EXAMPLE FRAME SIZES:
// - Mono 20ms: 960 samples * 2 bytes = 1920 bytes + 1 seq = 1921 bytes
// - Stereo 20ms: 960 samples * 2 channels * 2 bytes = 3840 bytes + 1 seq = 3841 bytes
//
// PARAMETERS:
//
//	pcmFrame: Raw int16 PCM samples in little-endian format
//	          Must contain (frameSamples * numChannels) samples
//	          Samples are interleaved: L, R, L, R, ... for stereo
//
// RETURNS:
//
//	error: Connection error, or nil if sent successfully
func (c *Client) SendRawAudioFrame(pcmFrame []int16) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or raw audio not enabled")
	}
	c.mu.RUnlock()

	msg := c.buildRawAudioPacket(pcmFrame)
	return c.sendMessageConnected(msg)
}

// buildAudioFrame constructs a complete Jamulus audio packet from Opus data
func (c *Client) buildAudioFrame(opusFrame []byte) []byte {
	// Audio packets are sent RAW without any protocol wrapper
	// Just pass through directly to the network
	return opusFrame
}

// buildRawAudioPacket constructs a raw audio packet from int16 PCM samples
func (c *Client) buildRawAudioPacket(pcmFrame []int16) []byte {
	expectedSamples := c.frameSizeSamples * c.numAudioChannels
	if len(pcmFrame) != expectedSamples {
		return nil // Error case, but we'll send empty
	}

	bufSize := len(pcmFrame) * 2
	c.mu.RLock()
	useSeqNum := c.useSequenceNumber
	seqNum := c.sequenceNumber
	if useSeqNum {
		c.sequenceNumber++
	}
	c.mu.RUnlock()

	// Allocate buffer for int16 samples
	if useSeqNum {
		bufSize++
	}
	buf := make([]byte, bufSize)
	buf[bufSize-1] = seqNum

	// Write int16 samples in little-endian format
	for i, sample := range pcmFrame {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(sample))
	}

	return buf
}

// SendChatMessage sends a chat message
func (c *Client) SendChatMessage(text string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	msg := c.buildChatMessage(text)
	return c.sendMessageConnected(msg)
}

// UpdateChannelInfo sends complete channel information to the server
func (c *Client) UpdateChannelInfo(info *ChannelInfo) error {
	if info == nil {
		return fmt.Errorf("channel info cannot be nil")
	}

	data := new(bytes.Buffer)

	// Country (2 bytes - little endian)
	binary.Write(data, binary.LittleEndian, uint16(getCountryIndex(info.Country)))

	// Instrument (4 bytes - little endian)
	binary.Write(data, binary.LittleEndian, uint32(info.Instrument))

	// Skill Level (1 byte) - 0=Beginner, 1=Intermediate, 2=Advanced, 3=Professional
	skillLevel := info.SkillLevel
	if skillLevel < 0 || skillLevel > 3 {
		skillLevel = 0
	}
	data.WriteByte(byte(skillLevel))

	// Name (UTF-8 string with 2-byte little-endian length prefix)
	nameBytes := []byte(info.Name)
	binary.Write(data, binary.LittleEndian, uint16(len(nameBytes)))
	data.Write(nameBytes)

	// City (UTF-8 string with 2-byte little-endian length prefix)
	cityBytes := []byte(info.City)
	binary.Write(data, binary.LittleEndian, uint16(len(cityBytes)))
	data.Write(cityBytes)

	return c.sendProtocolMessage(MsgTypeChannelInfo, data.Bytes())
}

// UpdateChannelName updates the client's channel name
func (c *Client) UpdateChannelName(name string) error {
	c.mu.Lock()
	c.clientName = name
	c.mu.Unlock()

	info := &ChannelInfo{
		ChannelID:  c.getClientID(),
		Name:       name,
		Country:    c.getCountryCode(),
		City:       c.getCity(),
		Instrument: c.getInstrument(),
		SkillLevel: c.getSkillLevel(),
	}

	return c.UpdateChannelInfo(info)
}

// UpdateChannelCountry updates the client's country
func (c *Client) UpdateChannelCountry(country string) error {
	c.mu.Lock()
	c.countryCode = country
	c.mu.Unlock()

	info := &ChannelInfo{
		ChannelID:  c.getClientID(),
		Name:       c.getClientName(),
		Country:    country,
		City:       c.getCity(),
		Instrument: c.getInstrument(),
		SkillLevel: c.getSkillLevel(),
	}

	return c.UpdateChannelInfo(info)
}

// UpdateChannelCity updates the client's city
func (c *Client) UpdateChannelCity(city string) error {
	c.mu.Lock()
	c.city = city
	c.mu.Unlock()

	info := &ChannelInfo{
		ChannelID:  c.getClientID(),
		Name:       c.getClientName(),
		Country:    c.getCountryCode(),
		City:       city,
		Instrument: c.getInstrument(),
		SkillLevel: c.getSkillLevel(),
	}

	return c.UpdateChannelInfo(info)
}

// UpdateChannelInstrument updates the client's instrument
func (c *Client) UpdateChannelInstrument(instrumentID int) error {
	c.mu.Lock()
	c.instrument = instrumentID
	c.mu.Unlock()

	info := &ChannelInfo{
		ChannelID:  c.getClientID(),
		Name:       c.getClientName(),
		Country:    c.getCountryCode(),
		City:       c.getCity(),
		Instrument: instrumentID,
		SkillLevel: c.getSkillLevel(),
	}

	return c.UpdateChannelInfo(info)
}

// UpdateChannelSkillLevel updates the client's skill level
// skillLevel: 0=Beginner, 1=Intermediate, 2=Advanced, 3=Professional
func (c *Client) UpdateChannelSkillLevel(skillLevel int) error {
	if skillLevel < 0 || skillLevel > 3 {
		return fmt.Errorf("skill level must be 0-3")
	}

	c.mu.Lock()
	c.skillLevel = skillLevel
	c.mu.Unlock()

	info := &ChannelInfo{
		ChannelID:  c.getClientID(),
		Name:       c.getClientName(),
		Country:    c.getCountryCode(),
		City:       c.getCity(),
		Instrument: c.getInstrument(),
		SkillLevel: skillLevel,
	}

	return c.UpdateChannelInfo(info)
}

// SetClientName sets the client's name
func (c *Client) SetClientName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientName = name
}

// SetCountryCode sets the client's country code
func (c *Client) SetCountryCode(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.countryCode = code
}

// SetCity sets the client's city
func (c *Client) SetCity(city string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.city = city
}

// SetInstrument sets the client's instrument
func (c *Client) SetInstrument(instrumentID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instrument = instrumentID
}

// SetSkillLevel sets the client's skill level
func (c *Client) SetSkillLevel(level int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skillLevel = level
}

// SetAudioCodec sets the audio codec type (Opus, Opus64, or Raw)
func (c *Client) SetAudioCodec(codecType int) error {
	if codecType != AudioCodecOpus && codecType != AudioCodecOpus64 {
		return fmt.Errorf("invalid codec type: %d", codecType)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.audioCodecType = codecType
	return nil
}

// GetAudioCodec returns the current audio codec type
func (c *Client) GetAudioCodec() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.audioCodecType
}

// IsRawAudioEnabled returns true if raw audio codec is active
func (c *Client) IsRawAudioEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return true
}

// GetFrameSizeSamples returns the audio frame size in samples
func (c *Client) GetFrameSizeSamples() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.frameSizeSamples
}

// GetNumAudioChannels returns the number of audio channels
func (c *Client) GetNumAudioChannels() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.numAudioChannels
}

// ReceiveAudioFrame receives an audio frame (non-blocking)
func (c *Client) ReceiveAudioFrame() ([]byte, error) {
	select {
	case frame := <-c.audioFrameCh:
		return frame, nil
	default:
		return nil, fmt.Errorf("no frame available")
	}
}

// ReceiveAudioFrameTimeout receives an audio frame with timeout
func (c *Client) ReceiveAudioFrameTimeout(timeout time.Duration) ([]byte, error) {
	select {
	case frame := <-c.audioFrameCh:
		return frame, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("receive timeout")
	}
}

// SetOnChatReceived sets a callback for received chat messages
func (c *Client) SetOnChatReceived(callback func(string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChatReceived = callback
}

// SetOnConnectedClientsInfo sets a callback for connected clients info
func (c *Client) SetOnConnectedClientsInfo(callback func(*ConnectedClientsInfo)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConnectedClientsInfo = callback
}

// SetOnChannelGain sets a callback for channel gain changes
func (c *Client) SetOnChannelGain(callback func(int, float32)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChannelGain = callback
}

// SetOnChannelPan sets a callback for channel pan changes
func (c *Client) SetOnChannelPan(callback func(int, float32)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChannelPan = callback
}

// SetOnMuteStateChanged sets a callback for mute state changes
func (c *Client) SetOnMuteStateChanged(callback func(int, bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMuteStateChanged = callback
}

// SetOnServerFull sets a callback for server full notification
func (c *Client) SetOnServerFull(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onServerFull = callback
}

// SetOnPingReceived sets a callback for ping response
func (c *Client) SetOnPingReceived(callback func(int)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPingReceived = callback
}

// SetOnClientIDReceived sets a callback for client ID reception
func (c *Client) SetOnClientIDReceived(callback func(int)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onClientIDReceived = callback
}

// SetOnVersionAndOSReceived sets a callback for version/OS reception
func (c *Client) SetOnVersionAndOSReceived(callback func(byte, string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onVersionAndOSReceived = callback
}

// SetOnRawAudioSupported sets a callback for raw audio support notification
func (c *Client) SetOnRawAudioSupported(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onRawAudioSupported = callback
}

// --- Private methods ---

func (c *Client) receiveLoop() {
	defer c.wg.Done()

	buffer := make([]byte, 65536)

	for {
		select {
		case <-c.doneCh:
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := c.conn.Read(buffer)

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		c.handleMessage(buffer[:n])
	}
}

func (c *Client) handleMessage(data []byte) {
	if len(data) < MessageMinLength {
		return
	}

	// Parse message header (little-endian for multi-byte fields)
	tag := binary.LittleEndian.Uint16(data[0:2])
	msgID := binary.LittleEndian.Uint16(data[2:4])
	if tag != 0 || msgID == 0 {
		c.handleAudioPacket(data)
		return
	}

	counter := data[4]
	length := binary.LittleEndian.Uint16(data[5:7])

	if len(data) < int(9+length) {
		return
	}
	crc1 := binary.LittleEndian.Uint16(data[7+length : 9+length])
	crc2 := calculateCRC16(data[:7+length])
	if crc1 != crc2 {
		return
	}

	// Check if this is a connection-less message
	if isConnectionLessMessageID(int(msgID)) {
		c.handleConnectionLessMessage(int(msgID), data[7:7+length])
	} else {
		c.handleConnectedMessage(int(msgID), counter, data[7:7+length])
	}
}

func (c *Client) handleAudioPacket(data []byte) {
}

func (c *Client) handleConnectedMessage(msgID int, counter uint8, data []byte) {
	// Send acknowledgment for this message
	if msgID != MsgTypeAckn {
		c.sendAcknowledgment(msgID, counter)
	}

	// Process the message
	switch msgID {
	case MsgTypeChatText:
		c.handleChatMessage(data)
	case MsgTypeConnClientsList:
		c.handleConnectedClientsList(data)
	case MsgTypeChannelGain:
		c.handleChannelGain(data)
	case MsgTypeChannelPan:
		c.handleChannelPan(data)
	case MsgTypeMuteStateChanged:
		c.handleMuteStateChanged(data)
	case MsgTypeClientID:
		c.handleClientID(data)
	case MsgTypeVersionAndOS:
		c.handleVersionAndOS(data)
	case MsgTypeRawAudioSupported:
		c.handleRawAudioSupported()
	case MsgTypeAckn:
		// Acknowledgment message - no action needed
	case MsgTypeOpusSupported:
		// Server supports Opus codec
	case MsgTypeReqJitterBufSize:
		c.handleReqJitterBufSize()
	case MsgTypeReqChannelInfo:
		c.handleReqChannelInfo()
	case MsgTypeReqConnClientsList:
		// Request for connected clients list - server will send it
	case MsgTypeReqNetTransportProps:
		c.handleReqNetTransportProps()
	}
}

func (c *Client) handleConnectionLessMessage(msgID int, data []byte) {
	switch msgID {
	case MsgTypeCLServerFull:
		c.callbackOnServerFull()
	case MsgTypeCLPingMs:
		if len(data) >= 2 {
			pingTime := int(binary.LittleEndian.Uint16(data[:2]))
			c.callbackOnPingReceived(pingTime)
		}
	case MsgTypeCLPingWithNumClients:
		if len(data) >= 4 {
			pingTime := int(binary.LittleEndian.Uint16(data[:2]))
			c.callbackOnPingReceived(pingTime)
		}
	}
}

func (c *Client) handleChatMessage(data []byte) {
	if len(data) < 2 {
		return
	}

	textLen := int(binary.LittleEndian.Uint16(data[:2]))

	if len(data) < 2+textLen {
		return
	}

	text := string(data[2 : 2+textLen])

	c.callbackOnChatReceived(text)
}

func (c *Client) handleConnectedClientsList(data []byte) {
	if len(data) < 2 {
		return
	}

	numClients := int(binary.LittleEndian.Uint16(data[:2]))
	info := &ConnectedClientsInfo{
		Clients: make([]ClientInfo, 0, numClients),
	}

	offset := 2
	for i := 0; i < numClients && offset < len(data); i++ {
		if offset+4 > len(data) {
			break
		}

		channelID := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
		offset += 2

		nameLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
		offset += 2

		if offset+nameLen > len(data) {
			break
		}

		name := string(data[offset : offset+nameLen])
		offset += nameLen

		if offset+4 > len(data) {
			break
		}

		gain := float32(binary.LittleEndian.Uint16(data[offset:offset+2])) / 100.0
		offset += 2

		pan := float32(binary.LittleEndian.Uint16(data[offset:offset+2]))/50.0 - 1.0
		offset += 2

		isMuted := false
		if offset < len(data) {
			isMuted = data[offset] != 0
			offset++
		}

		info.Clients = append(info.Clients, ClientInfo{
			ChannelID: channelID,
			Name:      name,
			Gain:      gain,
			Pan:       pan,
			IsMuted:   isMuted,
		})
	}

	c.callbackOnConnectedClientsInfo(info)
}

func (c *Client) handleChannelGain(data []byte) {
	if len(data) < 4 {
		return
	}

	channelID := int(binary.LittleEndian.Uint16(data[:2]))
	gain := float32(binary.LittleEndian.Uint16(data[2:4])) / 100.0

	c.callbackOnChannelGain(channelID, gain)
}

func (c *Client) handleChannelPan(data []byte) {
	if len(data) < 4 {
		return
	}

	channelID := int(binary.LittleEndian.Uint16(data[:2]))
	pan := float32(binary.LittleEndian.Uint16(data[2:4]))/50.0 - 1.0

	c.callbackOnChannelPan(channelID, pan)
}

func (c *Client) handleMuteStateChanged(data []byte) {
	if len(data) < 3 {
		return
	}

	channelID := int(binary.LittleEndian.Uint16(data[:2]))
	isMuted := data[2] != 0

	c.callbackOnMuteStateChanged(channelID, isMuted)
}

func (c *Client) handleClientID(data []byte) {
	if len(data) < 1 {
		return
	}

	clientID := int(data[0])
	c.mu.Lock()
	c.clientID = clientID
	c.mu.Unlock()

	c.callbackOnClientIDReceived(clientID)
}

func (c *Client) handleVersionAndOS(data []byte) {
	if len(data) < 3 {
		return
	}

	osType := data[0]
	versionLen := int(binary.LittleEndian.Uint16(data[1:3]))
	if len(data) < 3+versionLen {
		return
	}

	version := string(data[3 : 3+versionLen])
	c.callbackOnVersionAndOSReceived(osType, version)
}

func (c *Client) handleRawAudioSupported() {
	c.callbackOnRawAudioSupported()
}

func (c *Client) handleReqJitterBufSize() {
	// Server is requesting jitter buffer size - send default size
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, uint16(2)) // Default size
	c.sendProtocolMessage(MsgTypeJitterBufSize, data.Bytes())
}

func (c *Client) handleReqChannelInfo() {
	// Server is requesting channel info - send client info
	data := new(bytes.Buffer)
	c.mu.RLock()
	clientName := c.clientName
	c.mu.RUnlock()

	data.WriteByte(byte(len(clientName)))
	data.WriteString(clientName)
	c.sendProtocolMessage(MsgTypeChannelInfo, data.Bytes())
}

func (c *Client) handleReqNetTransportProps() {
	// Server is requesting network transport properties
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, uint32(c.basePacketSize))      // Base packet size
	binary.Write(data, binary.LittleEndian, uint16(c.frameSizeFactor))     // Block size factor
	data.WriteByte(uint8(c.numAudioChannels))                              // Num audio channels
	binary.Write(data, binary.LittleEndian, uint32(SYSTEM_SAMPLE_RATE_HZ)) // Sample rate
	binary.Write(data, binary.LittleEndian, uint16(c.audioCodecType))      // Audio codec type
	flags := uint16(0)
	if c.useSequenceNumber {
		flags = uint16(1)
	}
	binary.Write(data, binary.LittleEndian, flags)     // Flags
	binary.Write(data, binary.LittleEndian, uint32(0)) // Options

	c.sendProtocolMessage(MsgTypeNetworkTransportProp, data.Bytes())
}

// --- Message building ---

func (c *Client) buildChatMessage(text string) []byte {
	data := new(bytes.Buffer)

	// Message text with 2-byte little-endian length prefix
	textBytes := []byte(text)
	binary.Write(data, binary.LittleEndian, uint16(len(textBytes)))
	data.Write(textBytes)

	return c.buildProtocolMessage(MsgTypeChatText, data.Bytes())
}

func (c *Client) buildClientIDMessage(clientID int, clientName string) []byte {
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, uint16(clientID))
	data.WriteByte(byte(len(clientName)))
	data.WriteString(clientName)
	return c.buildProtocolMessage(MsgTypeClientID, data.Bytes())
}

func (c *Client) buildProtocolMessage(msgID int, data []byte) []byte {
	c.mu.Lock()
	counter := c.messageCounter
	c.messageCounter++
	c.mu.Unlock()
	return c.buildProtocolMessageWithCounter(msgID, counter, data)
}

func (c *Client) buildProtocolMessageWithCounter(msgID int, counter uint8, data []byte) []byte {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, uint16(0))
	binary.Write(buf, binary.LittleEndian, uint16(msgID))
	buf.WriteByte(counter)
	binary.Write(buf, binary.LittleEndian, uint16(len(data)))

	buf.Write(data)

	// Calculate CRC-16 over header + data
	messageWithoutCRC := buf.Bytes()
	crc := calculateCRC16(messageWithoutCRC)

	// Append CRC (little-endian)
	binary.Write(buf, binary.LittleEndian, crc)

	return buf.Bytes()
}

// sendAcknowledgment sends an acknowledgment message for a received message
func (c *Client) sendAcknowledgment(msgID int, counter uint8) error {
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, uint16(msgID))

	msg := c.buildProtocolMessageWithCounter(MsgTypeAckn, counter, data.Bytes())
	return c.sendMessageConnected(msg)
}

// --- Sending messages ---

func (c *Client) sendMessageConnected(data []byte) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	_, err := c.conn.Write(data)
	return err
}

func (c *Client) sendProtocolMessage(msgID int, data []byte) error {
	msg := c.buildProtocolMessage(msgID, data)
	return c.sendMessageConnected(msg)
}

// --- Callbacks ---

func (c *Client) callbackOnChatReceived(msg string) {
	c.mu.RLock()
	callback := c.onChatReceived
	c.mu.RUnlock()

	if callback != nil {
		callback(msg)
	}
}

func (c *Client) callbackOnConnectedClientsInfo(info *ConnectedClientsInfo) {
	c.mu.RLock()
	callback := c.onConnectedClientsInfo
	c.mu.RUnlock()

	if callback != nil {
		callback(info)
	}
}

func (c *Client) callbackOnChannelGain(channelID int, gain float32) {
	c.mu.RLock()
	callback := c.onChannelGain
	c.mu.RUnlock()

	if callback != nil {
		callback(channelID, gain)
	}
}

func (c *Client) callbackOnChannelPan(channelID int, pan float32) {
	c.mu.RLock()
	callback := c.onChannelPan
	c.mu.RUnlock()

	if callback != nil {
		callback(channelID, pan)
	}
}

func (c *Client) callbackOnMuteStateChanged(channelID int, isMuted bool) {
	c.mu.RLock()
	callback := c.onMuteStateChanged
	c.mu.RUnlock()

	if callback != nil {
		callback(channelID, isMuted)
	}
}

func (c *Client) callbackOnServerFull() {
	c.mu.RLock()
	callback := c.onServerFull
	c.mu.RUnlock()

	if callback != nil {
		callback()
	}
}

func (c *Client) callbackOnPingReceived(pingTime int) {
	c.mu.RLock()
	callback := c.onPingReceived
	c.mu.RUnlock()

	if callback != nil {
		callback(pingTime)
	}
}

func (c *Client) callbackOnClientIDReceived(clientID int) {
	c.mu.RLock()
	callback := c.onClientIDReceived
	c.mu.RUnlock()

	if callback != nil {
		callback(clientID)
	}
}

func (c *Client) callbackOnVersionAndOSReceived(osType byte, version string) {
	c.mu.RLock()
	callback := c.onVersionAndOSReceived
	c.mu.RUnlock()

	if callback != nil {
		callback(osType, version)
	}
}

func (c *Client) callbackOnRawAudioSupported() {
	c.mu.RLock()
	callback := c.onRawAudioSupported
	c.mu.RUnlock()

	if callback != nil {
		callback()
	}
}

// --- Helper functions ---

// calculateCRC16 calculates CRC-16 as used in Jamulus protocol
// Based on the CCRC implementation from Jamulus source (util.cpp)
func calculateCRC16(data []byte) uint16 {
	crc := uint32(crcInitial)

	for _, b := range data {
		// Process each bit of the byte
		for i := uint32(0); i < 8; i++ {
			// Shift left
			crc <<= 1

			// Check if bit 16 shifted out
			if crc&crcBitOutMask != 0 {
				crc |= 1
			}

			// Add new data bit to LSB
			if (b & (1 << (7 - i))) != 0 {
				crc ^= 1
			}

			// Apply polynomial if LSB is set
			if (crc & 1) != 0 {
				crc ^= crcPolynomial
			}
		}
	}

	// Invert the result and mask to 16 bits
	return uint16((^crc) & 0xFFFF)
}

func isConnectionLessMessageID(msgID int) bool {
	return msgID >= 1000 && msgID < 2000
}

// getCountryIndex converts country name to Jamulus country index
func getCountryIndex(country string) uint16 {
	countryMap := map[string]uint16{
		"US": 1, "GB": 2, "DE": 3, "FR": 4, "IT": 5, "ES": 6, "CA": 7, "AU": 8, "JP": 9, "CN": 10,
		"BR": 11, "MX": 12, "IN": 13, "ZA": 14, "RU": 15, "NL": 16, "SE": 17, "NO": 18, "DK": 19, "FI": 20,
		"CH": 21, "AT": 22, "BE": 23, "PL": 24, "CZ": 25, "GR": 26, "PT": 27, "TR": 28, "KR": 29, "TW": 30,
		"HK": 31, "SG": 32, "MY": 33, "TH": 34, "PH": 35, "ID": 36, "VN": 37, "NZ": 38, "IE": 39, "IL": 40,
		"AE": 41, "CL": 42, "AR": 43, "CO": 44, "PE": 45,
	}

	if index, ok := countryMap[country]; ok {
		return index
	}
	return 0 // Default: Unknown
}

// Helper getter methods with mutex protection
func (c *Client) getClientID() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientID
}

func (c *Client) getClientName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientName
}

func (c *Client) getCountryCode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.countryCode
}

func (c *Client) getCity() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.city
}

func (c *Client) getInstrument() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.instrument
}

func (c *Client) getSkillLevel() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.skillLevel
}
