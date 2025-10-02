package integration

import (
	"math"
	"odysail-boat-viz/storage"
)

type BoomSenseMapper struct {
	buffer *storage.RingBuffer
}

func NewBoomSenseMapper(buffer *storage.RingBuffer) *BoomSenseMapper {
	return &BoomSenseMapper{
		buffer: buffer,
	}
}

// BoomSenseData matches the structure from main.go
type BoomSenseData struct {
	BoomAngle     float64 `json:"boom_angle"`
	RollRate      float64 `json:"roll_rate"`
	PitchRate     float64 `json:"pitch_rate"`
	YawRate       float64 `json:"yaw_rate"`
	MainsheetLoad float64 `json:"mainsheet_load"`
	VangLoad      float64 `json:"vang_load"`
	EventType     string  `json:"event_type"`
	Timestamp     int64   `json:"timestamp"`
	WindSpeed     float64 `json:"wind_speed"`
	WindAngle     float64 `json:"wind_angle"`
	BoatSpeed     float64 `json:"boat_speed"`
}

func (m *BoomSenseMapper) GetCurrentData() BoomSenseData {
	data := BoomSenseData{
		EventType: "normal",
		Timestamp: 0,
	}

	// PGN 127257 - Attitude (heel angle, pitch, yaw)
	if msg := m.buffer.GetLatestByPGN(127257); msg != nil {
		if heelAngle, ok := msg.Fields["heel_angle"].(float64); ok {
			// Heel angle is already in degrees
			data.BoomAngle = heelAngle // Temporary: using heel as rough boom estimate
		}
		if pitch, ok := msg.Fields["pitch_deg"].(float64); ok {
			data.PitchRate = pitch
		}
		if yaw, ok := msg.Fields["yaw_deg"].(float64); ok {
			data.YawRate = yaw
		}
		data.Timestamp = msg.Timestamp.UnixMilli()
	}

	// PGN 127251 - Rate of Turn
	if msg := m.buffer.GetLatestByPGN(127251); msg != nil {
		if rot, ok := msg.Fields["rate_of_turn_deg_s"].(float64); ok {
			data.RollRate = rot
		}
	}

	// PGN 130306 - Wind Data
	if msg := m.buffer.GetLatestByPGN(130306); msg != nil {
		if ws, ok := msg.Fields["wind_speed_kts"].(float64); ok {
			data.WindSpeed = ws
		}
		if wa, ok := msg.Fields["wind_angle_deg"].(float64); ok {
			data.WindAngle = wa
		}
		if data.Timestamp == 0 {
			data.Timestamp = msg.Timestamp.UnixMilli()
		}
	}

	// PGN 129026 - COG & SOG (boat speed)
	if msg := m.buffer.GetLatestByPGN(129026); msg != nil {
		if sog, ok := msg.Fields["sog_kts"].(float64); ok {
			data.BoatSpeed = sog
		}
	}

	// PGN 128259 - Speed Water Referenced (alternative)
	if data.BoatSpeed == 0 {
		if msg := m.buffer.GetLatestByPGN(128259); msg != nil {
			if ws, ok := msg.Fields["water_speed_kts"].(float64); ok {
				data.BoatSpeed = ws
			}
		}
	}

	return data
}

// GetHeelAngle returns current heel angle in degrees
func (m *BoomSenseMapper) GetHeelAngle() float64 {
	if msg := m.buffer.GetLatestByPGN(127257); msg != nil {
		if heel, ok := msg.Fields["heel_angle"].(float64); ok {
			return heel
		}
	}
	return 0.0
}

// GetWindData returns wind speed (kts) and angle (degrees)
func (m *BoomSenseMapper) GetWindData() (speed, angle float64) {
	if msg := m.buffer.GetLatestByPGN(130306); msg != nil {
		if ws, ok := msg.Fields["wind_speed_kts"].(float64); ok {
			speed = ws
		}
		if wa, ok := msg.Fields["wind_angle_deg"].(float64); ok {
			angle = wa
		}
	}
	return
}

// GetBoatSpeed returns current boat speed in knots
func (m *BoomSenseMapper) GetBoatSpeed() float64 {
	// Try COG/SOG first
	if msg := m.buffer.GetLatestByPGN(129026); msg != nil {
		if sog, ok := msg.Fields["sog_kts"].(float64); ok {
			return sog
		}
	}
	
	// Fallback to water speed
	if msg := m.buffer.GetLatestByPGN(128259); msg != nil {
		if ws, ok := msg.Fields["water_speed_kts"].(float64); ok {
			return ws
		}
	}
	
	return 0.0
}

// CalculateApparentWind computes apparent wind from true wind + boat speed
func (m *BoomSenseMapper) CalculateApparentWind() (aws, awa float64) {
	tws, twa := m.GetWindData()
	bs := m.GetBoatSpeed()
	
	if tws == 0 {
		return 0, 0
	}
	
	// Convert to radians
	twaRad := twa * math.Pi / 180.0
	
	// Vector calculation
	// True wind components
	twx := tws * math.Sin(twaRad)
	twy := tws * math.Cos(twaRad)
	
	// Apparent wind = true wind - boat velocity
	awx := twx
	awy := twy - bs
	
	// Apparent wind speed
	aws = math.Sqrt(awx*awx + awy*awy)
	
	// Apparent wind angle
	awa = math.Atan2(awx, awy) * 180.0 / math.Pi
	
	// Normalize to 0-180 range
	if awa < 0 {
		awa = -awa
	}
	
	return
}