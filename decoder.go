package nmea

import (
	"encoding/binary"
	"math"
)

// Decoder handles PGN decoding
type Decoder struct {
	handlers map[int]DecoderFunc
}

type DecoderFunc func(data []byte) (map[string]interface{}, error)

func NewDecoder() *Decoder {
	d := &Decoder{
		handlers: make(map[int]DecoderFunc),
	}
	d.registerDefaultHandlers()
	return d
}

func (d *Decoder) Decode(pgn int, data []byte) (map[string]interface{}, error) {
	if handler, ok := d.handlers[pgn]; ok {
		return handler(data)
	}
	return nil, nil // No handler for this PGN
}

func (d *Decoder) registerDefaultHandlers() {
	// Critical PGNs for sailing/BoomSense
	d.handlers[127257] = decodePGN127257 // Attitude (CRITICAL for heel angle)
	d.handlers[127251] = decodePGN127251 // Rate of Turn
	d.handlers[130306] = decodePGN130306 // Wind Data (CRITICAL)
	d.handlers[127250] = decodePGN127250 // Vessel Heading
	d.handlers[129026] = decodePGN129026 // COG & SOG (CRITICAL for boat speed)
	d.handlers[129025] = decodePGN129025 // Position Rapid Update
	d.handlers[129029] = decodePGN129029 // GNSS Position Data
	d.handlers[128267] = decodePGN128267 // Water Depth
	d.handlers[128259] = decodePGN128259 // Speed Water Referenced
	d.handlers[128275] = decodePGN128275 // Distance Log
	d.handlers[127245] = decodePGN127245 // Rudder
	d.handlers[127237] = decodePGN127237 // Heading/Track Control
	d.handlers[129284] = decodePGN129284 // Navigation Data
	d.handlers[129540] = decodePGN129540 // GNSS Satellites
	d.handlers[126992] = decodePGN126992 // System Time
	d.handlers[127508] = decodePGN127508 // Battery Status
	d.handlers[127489] = decodePGN127489 // Engine Parameters
	d.handlers[130310] = decodePGN130310 // Environmental Parameters
	d.handlers[130312] = decodePGN130312 // Temperature
	d.handlers[130313] = decodePGN130313 // Humidity
}

// Helper functions for reading multi-byte values
func u8(data []byte, offset int) uint8 {
	if offset >= len(data) {
		return 0xFF
	}
	return data[offset]
}

func u16le(data []byte, offset int) uint16 {
	if offset+1 >= len(data) {
		return 0xFFFF
	}
	return binary.LittleEndian.Uint16(data[offset : offset+2])
}

func u32le(data []byte, offset int) uint32 {
	if offset+3 >= len(data) {
		return 0xFFFFFFFF
	}
	return binary.LittleEndian.Uint32(data[offset : offset+4])
}

func i8(data []byte, offset int) int8 {
	if offset >= len(data) {
		return 0x7F
	}
	return int8(data[offset])
}

func i16le(data []byte, offset int) int16 {
	if offset+1 >= len(data) {
		return 0x7FFF
	}
	return int16(binary.LittleEndian.Uint16(data[offset : offset+2]))
}

func i32le(data []byte, offset int) int32 {
	if offset+3 >= len(data) {
		return 0x7FFFFFFF
	}
	return int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
}

func i64le(data []byte, offset int) int64 {
	if offset+7 >= len(data) {
		return 0x7FFFFFFFFFFFFFFF
	}
	return int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
}

// === CRITICAL: PGN 127257 - Attitude (Yaw, Pitch, Roll) ===
// This provides heel angle for BoomSense!
func decodePGN127257(data []byte) (map[string]interface{}, error) {
	if len(data) < 7 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	yawRaw := i16le(data, 1)
	pitchRaw := i16le(data, 3)
	rollRaw := i16le(data, 5)

	result["sid"] = sid

	if yawRaw != 0x7FFF {
		yaw := float64(yawRaw) * 0.0001 // radians
		result["yaw_rad"] = yaw
		result["yaw_deg"] = yaw * 180.0 / math.Pi
	}

	if pitchRaw != 0x7FFF {
		pitch := float64(pitchRaw) * 0.0001 // radians
		result["pitch_rad"] = pitch
		result["pitch_deg"] = pitch * 180.0 / math.Pi
	}

	if rollRaw != 0x7FFF {
		roll := float64(rollRaw) * 0.0001 // radians (heel angle)
		result["roll_rad"] = roll
		result["roll_deg"] = roll * 180.0 / math.Pi
		result["heel_angle"] = roll * 180.0 / math.Pi // Alias for clarity
	}

	return result, nil
}

// === CRITICAL: PGN 130306 - Wind Data ===
func decodePGN130306(data []byte) (map[string]interface{}, error) {
	if len(data) < 6 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	wsRaw := u16le(data, 1)
	waRaw := u16le(data, 3)
	ref := u8(data, 5)

	result["sid"] = sid
	result["wind_reference"] = ref

	if wsRaw != 0xFFFF {
		windSpeed := float64(wsRaw) * 0.01 // m/s
		result["wind_speed_ms"] = windSpeed
		result["wind_speed_kts"] = windSpeed * 1.94384 // Convert to knots
	}

	if waRaw != 0xFFFF {
		windAngle := float64(waRaw) * 0.0001 // radians
		result["wind_angle_rad"] = windAngle
		result["wind_angle_deg"] = windAngle * 180.0 / math.Pi
	}

	return result, nil
}

// === CRITICAL: PGN 129026 - COG & SOG Rapid Update ===
func decodePGN129026(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	cogRaw := u16le(data, 1)
	sogRaw := u16le(data, 3)

	result["sid"] = sid

	if cogRaw != 0xFFFF {
		cog := float64(cogRaw) * 0.0001 // radians
		result["cog_rad"] = cog
		result["cog_deg"] = cog * 180.0 / math.Pi
	}

	if sogRaw != 0xFFFF {
		sog := float64(sogRaw) * 0.01 // m/s
		result["sog_ms"] = sog
		result["sog_kts"] = sog * 1.94384
	}

	return result, nil
}

// === PGN 127250 - Vessel Heading ===
func decodePGN127250(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	headingRaw := u16le(data, 1)
	deviationRaw := i16le(data, 3)
	variationRaw := i16le(data, 5)
	ref := u8(data, 7)

	result["sid"] = sid
	result["heading_reference"] = ref

	if headingRaw != 0xFFFF {
		heading := float64(headingRaw) * 0.0001
		result["heading_rad"] = heading
		result["heading_deg"] = heading * 180.0 / math.Pi
	}

	if deviationRaw != 0x7FFF {
		deviation := float64(deviationRaw) * 0.0001
		result["deviation_rad"] = deviation
		result["deviation_deg"] = deviation * 180.0 / math.Pi
	}

	if variationRaw != 0x7FFF {
		variation := float64(variationRaw) * 0.0001
		result["variation_rad"] = variation
		result["variation_deg"] = variation * 180.0 / math.Pi
	}

	return result, nil
}

// === PGN 127251 - Rate of Turn ===
func decodePGN127251(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// 8-byte variant
	if len(data) >= 8 {
		sid := u8(data, 0)
		rotRaw := i32le(data, 1)
		result["sid"] = sid

		if rotRaw != 0x7FFFFFFF {
			rot := float64(rotRaw) * 3.125e-8 // rad/s
			result["rate_of_turn_rad_s"] = rot
			result["rate_of_turn_deg_s"] = rot * 180.0 / math.Pi
		}
		return result, nil
	}

	// 3-byte variant
	if len(data) >= 3 {
		sid := u8(data, 0)
		rotRaw := i16le(data, 1)
		result["sid"] = sid

		if rotRaw != 0x7FFF {
			rot := float64(rotRaw) * 0.0001
			result["rate_of_turn_rad_s"] = rot
			result["rate_of_turn_deg_s"] = rot * 180.0 / math.Pi
		}
		return result, nil
	}

	return nil, nil
}

// === PGN 129025 - Position Rapid Update ===
func decodePGN129025(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	latRaw := u32le(data, 0)
	lonRaw := u32le(data, 4)

	if latRaw != 0xFFFFFFFF {
		lat := (float64(latRaw) - 0x80000000) * 1e-7
		result["latitude"] = lat
	}

	if lonRaw != 0xFFFFFFFF {
		lon := (float64(lonRaw) - 0x80000000) * 1e-7
		result["longitude"] = lon
	}

	return result, nil
}

// === PGN 128267 - Water Depth ===
func decodePGN128267(data []byte) (map[string]interface{}, error) {
	if len(data) < 5 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	depthRaw := u32le(data, 1)

	result["sid"] = sid

	if depthRaw != 0xFFFFFFFF {
		result["depth_m"] = float64(depthRaw) * 0.01
	}

	return result, nil
}

// === PGN 128259 - Speed Water Referenced ===
func decodePGN128259(data []byte) (map[string]interface{}, error) {
	if len(data) < 7 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	waterRaw := u16le(data, 1)
	groundRaw := u16le(data, 3)

	result["sid"] = sid

	if waterRaw != 0xFFFF {
		ws := float64(waterRaw) * 0.01
		result["water_speed_ms"] = ws
		result["water_speed_kts"] = ws * 1.94384
	}

	if groundRaw != 0xFFFF {
		gs := float64(groundRaw) * 0.01
		result["ground_speed_ms"] = gs
		result["ground_speed_kts"] = gs * 1.94384
	}

	return result, nil
}

// === PGN 128275 - Distance Log ===
func decodePGN128275(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	logRaw := u32le(data, 0)
	tripRaw := u32le(data, 4)

	if logRaw != 0xFFFFFFFF {
		result["log_distance_m"] = float64(logRaw) * 185.2 // 0.1 nm to meters
	}

	if tripRaw != 0xFFFFFFFF {
		result["trip_distance_m"] = float64(tripRaw) * 185.2
	}

	return result, nil
}

// More decoders continued in Part 2...