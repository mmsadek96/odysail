package nmea

import (
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"odysail-boat-viz/storage"
)

type Collector struct {
	config      Config
	client      mqtt.Client
	decoder     *Decoder
	buffer      BufferInterface
	csvWriter   CSVWriterInterface
	stats       *Statistics
	rawFrames   chan RawFrame
	decodedData chan DecodedMessage
	done        chan struct{}
}

// Interfaces for dependency injection (testing)
type BufferInterface interface {
	Push(msg storage.DecodedMessage)
	GetLatestByPGN(pgn int) *storage.DecodedMessage
	Size() int
	GetStats() map[string]interface{}
}

type CSVWriterInterface interface {
	WriteDecoded(msg storage.DecodedMessage)
	Close()
}

func NewCollector(config Config, buffer BufferInterface, csvWriter CSVWriterInterface) *Collector {
	return &Collector{
		config:      config,
		decoder:     NewDecoder(),
		buffer:      buffer,
		csvWriter:   csvWriter,
		stats:       NewStatistics(),
		rawFrames:   make(chan RawFrame, config.QueueSize),
		decodedData: make(chan DecodedMessage, config.QueueSize),
		done:        make(chan struct{}),
	}
}

func (c *Collector) Start() error {
	log.Printf("[NMEA] Starting collector...")
	log.Printf("[NMEA] Config: Broker=%s:%d Topic=%s", c.config.MQTTBroker, c.config.MQTTPort, c.config.MQTTTopic)

	// Setup MQTT client options
	opts := mqtt.NewClientOptions()

	// Broker URL
	protocol := "tcp"
	if c.config.UseTLS {
		protocol = "tls"
	}
	brokerURL := fmt.Sprintf("%s://%s:%d", protocol, c.config.MQTTBroker, c.config.MQTTPort)
	opts.AddBroker(brokerURL)

	// Client ID
	clientID := fmt.Sprintf("odysail-collector-%d", time.Now().Unix())
	opts.SetClientID(clientID)

	// Credentials
	if c.config.MQTTUsername != "" {
		opts.SetUsername(c.config.MQTTUsername)
		opts.SetPassword(c.config.MQTTPassword)
	}

	// TLS config
	if c.config.UseTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: c.config.InsecureSkipTLS,
		}
		opts.SetTLSConfig(tlsConfig)
	}

	// Connection settings
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)

	// Callbacks
	opts.OnConnect = c.onConnect
	opts.OnConnectionLost = c.onConnectionLost
	opts.OnReconnecting = c.onReconnecting

	// Create and connect client
	c.client = mqtt.NewClient(opts)

	log.Printf("[MQTT] Connecting to %s as %s...", brokerURL, clientID)

	token := c.client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("MQTT connect timeout")
	}
	if token.Error() != nil {
		return fmt.Errorf("MQTT connect failed: %w", token.Error())
	}

	// Start worker goroutines
	log.Printf("[NMEA] Starting %d decoder workers", c.config.DecoderWorkers)
	for i := 0; i < c.config.DecoderWorkers; i++ {
		go c.decodeWorker(i)
	}
	go c.storageWorker()
	go c.statsReporter()

	log.Printf("[NMEA] Collector started successfully")
	return nil
}

func (c *Collector) Stop() {
	log.Printf("[NMEA] Stopping collector...")
	close(c.done)

	if c.client != nil && c.client.IsConnected() {
		c.client.Disconnect(1000)
	}

	if c.csvWriter != nil {
		c.csvWriter.Close()
	}

	successRate := 0.0
	if c.stats.MessagesProcessed > 0 {
		successRate = float64(c.stats.DecodeSuccesses) / float64(c.stats.MessagesProcessed) * 100.0
	}

	log.Printf("[NMEA] Collector stopped - processed %d messages (%.1f%% decode success)",
		c.stats.MessagesProcessed, successRate)
}

func (c *Collector) onConnect(client mqtt.Client) {
	log.Printf("[MQTT] Connected successfully")

	token := client.Subscribe(c.config.MQTTTopic, 0, c.onMessage)
	if !token.WaitTimeout(5 * time.Second) {
		log.Printf("[MQTT] Subscribe timeout for %s", c.config.MQTTTopic)
		return
	}
	if token.Error() != nil {
		log.Printf("[MQTT] Subscribe error: %v", token.Error())
		return
	}

	log.Printf("[MQTT] Subscribed to %s", c.config.MQTTTopic)
}

func (c *Collector) onConnectionLost(client mqtt.Client, err error) {
	log.Printf("[MQTT] Connection lost: %v (will auto-reconnect)", err)
}

func (c *Collector) onReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	log.Printf("[MQTT] Reconnecting...")
}

func (c *Collector) onMessage(client mqtt.Client, msg mqtt.Message) {
	// Parse JSON payload
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		// Not JSON, skip
		return
	}

	// Parse raw frame
	frame := c.parseRawFrame(msg.Topic(), payload)
	if frame == nil {
		return
	}

	// Send to decoder workers
	select {
	case c.rawFrames <- *frame:
		// Success
	case <-c.done:
		return
	default:
		// Queue full, drop message (prioritize latest data)
	}
}

func (c *Collector) parseRawFrame(topic string, payload map[string]interface{}) *RawFrame {
	frame := &RawFrame{
		Timestamp: time.Now(),
		Topic:     topic,
	}

	// Extract timestamp
	if ts, ok := payload["ts"].(float64); ok {
		frame.Timestamp = time.Unix(0, int64(ts)*1e6)
	} else if ts, ok := payload["timestamp"].(float64); ok {
		frame.Timestamp = time.Unix(0, int64(ts)*1e6)
	}

	// Extract PGN
	if pgn, ok := payload["pgn"].(float64); ok {
		frame.PGN = int(pgn)
	} else {
		// Try to compute from CAN ID components
		dp, _ := payload["dp"].(float64)
		pf, _ := payload["pf"].(float64)
		ps, _ := payload["ps"].(float64)
		frame.PGN = PGNFromParts(uint8(dp), uint8(pf), uint8(ps))
	}

	// Extract source address
	if src, ok := payload["src"].(float64); ok {
		frame.Source = uint8(src)
	} else if id, ok := payload["id"].(float64); ok {
		frame.Source = uint8(int(id) & 0xFF)
	}

	// Extract data
	if dataStr, ok := payload["data"].(string); ok {
		frame.Data = c.parseHexData(dataStr)
	} else if dataArr, ok := payload["data"].([]interface{}); ok {
		frame.Data = c.parseArrayData(dataArr)
	} else {
		return nil // No data, invalid frame
	}

	if len(frame.Data) == 0 {
		return nil
	}

	frame.Length = len(frame.Data)

	return frame
}

func (c *Collector) parseHexData(dataStr string) []byte {
	// Remove common separators
	cleaned := strings.ReplaceAll(dataStr, " ", "")
	cleaned = strings.ReplaceAll(cleaned, ",", "")
	cleaned = strings.ReplaceAll(cleaned, ":", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ToLower(cleaned)

	// Decode hex
	data, err := hex.DecodeString(cleaned)
	if err != nil {
		return nil
	}

	return data
}

func (c *Collector) parseArrayData(dataArr []interface{}) []byte {
	data := make([]byte, 0, len(dataArr))
	for _, v := range dataArr {
		if num, ok := v.(float64); ok {
			data = append(data, byte(num))
		}
	}
	return data
}

func (c *Collector) decodeWorker(id int) {
	log.Printf("[NMEA] Decoder worker %d started", id)

	for {
		select {
		case frame := <-c.rawFrames:
			// Decode the frame
			fields, err := c.decoder.Decode(frame.PGN, frame.Data)

			// Build decoded message
			decoded := DecodedMessage{
				Timestamp:   frame.Timestamp,
				PGN:         frame.PGN,
				PGNName:     GetPGNName(frame.PGN),
				Source:      frame.Source,
				Measurement: GetMeasurementType(frame.PGN),
				Fields:      fields,
				Raw:         frame.Data,
			}

			// Record statistics
			success := err == nil && fields != nil && len(fields) > 0
			c.stats.RecordMessage(frame.PGN, decoded.Measurement, success)

			// Send to storage
			select {
			case c.decodedData <- decoded:
				// Success
			case <-c.done:
				return
			default:
				// Storage queue full, drop
			}

		case <-c.done:
			log.Printf("[NMEA] Decoder worker %d stopped", id)
			return
		}
	}
}

func (c *Collector) storageWorker() {
	log.Printf("[NMEA] Storage worker started")

	for {
		select {
		case msg := <-c.decodedData:
			// Convert to storage.DecodedMessage
			storageMsg := storage.DecodedMessage{
				Timestamp:   msg.Timestamp,
				PGN:         msg.PGN,
				PGNName:     msg.PGNName,
				Source:      msg.Source,
				Measurement: msg.Measurement,
				Fields:      msg.Fields,
				Raw:         msg.Raw,
			}

			// Store in ring buffer
			if c.buffer != nil {
				c.buffer.Push(storageMsg)
			}

			// Write to CSV if enabled
			if c.csvWriter != nil {
				c.csvWriter.WriteDecoded(storageMsg)
			}

		case <-c.done:
			log.Printf("[NMEA] Storage worker stopped")
			return
		}
	}
}

func (c *Collector) statsReporter() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := c.stats.GetSnapshot()
			log.Printf("[NMEA] Stats: %d msgs, %.1f msg/s, %.1f%% success, buffer: %d",
				stats["messages_processed"],
				stats["messages_per_sec"],
				stats["success_rate"],
				c.buffer.Size())

		case <-c.done:
			return
		}
	}
}

func (c *Collector) Buffer() BufferInterface {
	return c.buffer
}

func (c *Collector) Stats() *Statistics {
	return c.stats
}

func (c *Collector) IsConnected() bool {
	return c.client != nil && c.client.IsConnected()
}