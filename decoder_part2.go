package nmea

import (
	"math"
	"time"
)

// === PGN 129029 - GNSS Position Data ===
func decodePGN129029(data []byte) (map[string]interface{}, error) {
	if len(data) < 43 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	dateDays := u16le(data, 1)
	timeRaw := u32le(data, 3)
	latRaw := i64le(data, 7)
	lonRaw := i64le(data, 15)
	altRaw := i64le(data, 23)

	pack1 := u8(data, 31)
	gnssType := pack1 & 0x0F
	method := (pack1 >> 4) & 0x0F

	pack2 := u8(data, 32)
	integrity := pack2 & 0b11

	svs := u8(data, 33)
	hdopRaw := i16le(data, 34)
	pdopRaw := i16le(data, 36)
	geoidRaw := i32le(data, 38)
	refStations := u8(data, 42)

	result["sid"] = sid

	// Build UTC timestamp
	if dateDays != 0xFFFF && timeRaw != 0xFFFFFFFF {
		midnight := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(dateDays))
		fixTime := midnight.Add(time.Duration(float64(timeRaw)*0.0001) * time.Second)
		result["fix_time_utc"] = fixTime.Format(time.RFC3339)
	}

	if latRaw != 0x7FFFFFFFFFFFFFFF {
		result["latitude"] = float64(latRaw) * 1e-16
	}

	if lonRaw != 0x7FFFFFFFFFFFFFFF {
		result["longitude"] = float64(lonRaw) * 1e-16
	}

	if altRaw != 0x7FFFFFFFFFFFFFFF {
		result["altitude_m"] = float64(altRaw) * 1e-6
	}

	result["gnss_type"] = gnssType
	result["method"] = method
	result["integrity"] = integrity
	result["satellites"] = svs

	if hdopRaw != 0x7FFF {
		result["hdop"] = float64(hdopRaw) * 0.01
	}

	if pdopRaw != 0x7FFF {
		result["pdop"] = float64(pdopRaw) * 0.01
	}

	if geoidRaw != 0x7FFFFFFF {
		result["geoidal_separation_m"] = float64(geoidRaw) * 0.01
	}

	result["reference_stations"] = refStations

	return result, nil
}

// === PGN 127245 - Rudder ===
func decodePGN127245(data []byte) (map[string]interface{}, error) {
	if len(data) < 6 {
		return nil, nil
	}

	result := make(map[string]interface{})
	instance := u8(data, 0)
	directionOrder := u8(data, 1)
	angleOrderRaw := i16le(data, 2)
	positionRaw := i16le(data, 4)

	result["rudder_instance"] = instance
	result["direction_order"] = directionOrder

	if angleOrderRaw != 0x7FFF {
		angle := float64(angleOrderRaw) * 0.0001
		result["rudder_angle_order_rad"] = angle
		result["rudder_angle_order_deg"] = angle * 180.0 / math.Pi
	}

	if positionRaw != 0x7FFF {
		pos := float64(positionRaw) * 0.0001
		result["rudder_position_rad"] = pos
		result["rudder_position_deg"] = pos * 180.0 / math.Pi
	}

	return result, nil
}

// === PGN 127237 - Heading/Track Control (Autopilot) ===
func decodePGN127237(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})

	b0 := u8(data, 0)
	b1 := u8(data, 1)
	b2 := u8(data, 2)

	result["rudder_limit_exceeded"] = (b0 >> 6) & 0b11
	result["off_heading_exceeded"] = (b0 >> 4) & 0b11
	result["off_track_exceeded"] = (b0 >> 2) & 0b11
	result["override"] = b0 & 0b11
	result["steering_mode"] = (b1 >> 5) & 0b111
	result["turn_mode"] = (b1 >> 2) & 0b111
	result["heading_reference"] = ((b1 & 0b11) | ((b2 >> 7) & 0b1) << 2)
	result["commanded_rudder_direction"] = b2 & 0b111

	offset := 3
	cmdRudderAngleRaw := i16le(data, offset)
	offset += 2
	headingToSteerRaw := u16le(data, offset)
	offset += 2
	trackRaw := u16le(data, offset)
	offset += 2

	if cmdRudderAngleRaw != 0x7FFF {
		result["commanded_rudder_angle_rad"] = float64(cmdRudderAngleRaw) * 0.0001
	}

	if headingToSteerRaw != 0xFFFF {
		result["heading_to_steer_rad"] = float64(headingToSteerRaw) * 0.0001
	}

	if trackRaw != 0xFFFF {
		result["track_rad"] = float64(trackRaw) * 0.0001
	}

	return result, nil
}

// === PGN 129284 - Navigation Data ===
func decodePGN129284(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	offset := 0

	sid := u8(data, offset)
	offset++
	distCm := u32le(data, offset)
	offset += 4
	flags := u8(data, offset)
	offset++

	result["sid"] = sid

	if distCm != 0xFFFFFFFF {
		result["distance_to_waypoint_m"] = float64(distCm) / 100.0
	}

	result["bearing_reference"] = (flags >> 6) & 0b11
	result["perpendicular_crossed"] = (flags >> 4) & 0b11
	result["arrival_circle_entered"] = (flags >> 2) & 0b11
	result["calculation_type"] = flags & 0b11

	if offset+4 <= len(data) {
		etaTimeRaw := u32le(data, offset)
		offset += 4
		etaDateRaw := u16le(data, offset)
		offset += 2

		if etaDateRaw != 0xFFFF && etaTimeRaw != 0xFFFFFFFF {
			midnight := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(etaDateRaw))
			eta := midnight.Add(time.Duration(float64(etaTimeRaw)*0.0001) * time.Second)
			result["eta_utc"] = eta.Format(time.RFC3339)
		}
	}

	return result, nil
}

// === PGN 129540 - GNSS Satellites in View ===
func decodePGN129540(data []byte) (map[string]interface{}, error) {
	if len(data) < 3 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	hdr := u8(data, 1)
	satsInView := u8(data, 2)

	result["sid"] = sid
	result["range_residual_mode"] = (hdr >> 6) & 0b11
	result["sats_in_view"] = satsInView

	offset := 3
	for i := 1; i <= int(satsInView) && offset+9 <= len(data); i++ {
		prn := u8(data, offset)
		offset++
		elevRaw := i16le(data, offset)
		offset += 2
		azimRaw := u16le(data, offset)
		offset += 2
		snrRaw := i16le(data, offset)
		offset += 2
		rngRaw := u32le(data, offset)
		offset += 4
		status := u8(data, offset)
		offset++

		if prn != 0xFF {
			result[formatSatField("prn", i)] = prn
		}
		if elevRaw != 0x7FFF {
			result[formatSatField("elevation_rad", i)] = float64(elevRaw) * 0.0001
		}
		if azimRaw != 0xFFFF {
			result[formatSatField("azimuth_rad", i)] = float64(azimRaw) * 0.0001
		}
		if snrRaw != 0x7FFF {
			result[formatSatField("snr_dbhz", i)] = float64(snrRaw) * 0.1
		}
		if rngRaw != 0xFFFFFFFF {
			result[formatSatField("range_residual_m", i)] = float64(rngRaw) * 0.001
		}
		result[formatSatField("status", i)] = (status >> 4) & 0x0F
	}

	return result, nil
}

func formatSatField(field string, index int) string {
	return "sv_" + string(rune('0'+index)) + "_" + field
}

// === PGN 126992 - System Time ===
func decodePGN126992(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	timeSource := u8(data, 1)
	days := u16le(data, 2)
	ms := u32le(data, 4)

	result["sid"] = sid
	result["time_source"] = timeSource

	if days != 0xFFFF {
		date := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(days))
		result["date"] = date.Format("2006-01-02")
	}

	if ms != 0xFFFFFFFF {
		seconds := float64(ms) * 0.0001
		hours := int(seconds / 3600)
		minutes := int((seconds - float64(hours*3600)) / 60)
		secs := seconds - float64(hours*3600) - float64(minutes*60)
		result["time_of_day"] = formatTime(hours, minutes, secs)
	}

	return result, nil
}

func formatTime(h, m int, s float64) string {
	return time.Date(0, 1, 1, h, m, int(s), int((s-float64(int(s)))*1e9), time.UTC).Format("15:04:05.000")
}

// === PGN 127508 - Battery Status ===
func decodePGN127508(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	instance := u8(data, 0)
	voltageRaw := u16le(data, 1)
	currentRaw := i16le(data, 3)
	tempRaw := u16le(data, 5)
	sid := u8(data, 7)

	result["battery_instance"] = instance
	result["sid"] = sid

	if voltageRaw != 0xFFFF {
		result["battery_voltage_v"] = float64(voltageRaw) * 0.01
	}

	if currentRaw != 0x7FFF {
		result["battery_current_a"] = float64(currentRaw) * 0.1
	}

	if tempRaw != 0xFFFF {
		result["battery_temperature_c"] = float64(tempRaw)*0.01 - 273.15
	}

	return result, nil
}

// === PGN 127489 - Engine Parameters Dynamic ===
func decodePGN127489(data []byte) (map[string]interface{}, error) {
	if len(data) < 8 {
		return nil, nil
	}

	result := make(map[string]interface{})
	instance := u8(data, 0)
	oilPressureRaw := u16le(data, 1)
	oilTempRaw := u16le(data, 3)
	engineTempRaw := u16le(data, 5)

	result["engine_instance"] = instance

	if oilPressureRaw != 0xFFFF {
		result["oil_pressure_pa"] = float64(oilPressureRaw) * 100
	}

	if oilTempRaw != 0xFFFF {
		result["oil_temperature_c"] = float64(oilTempRaw)*0.1 - 273.15
	}

	if engineTempRaw != 0xFFFF {
		result["engine_temperature_c"] = float64(engineTempRaw)*0.01 - 273.15
	}

	if len(data) >= 9 {
		altVoltageRaw := u16le(data, 7)
		if altVoltageRaw != 0xFFFF {
			result["alternator_voltage_v"] = float64(altVoltageRaw) * 0.01
		}
	}

	return result, nil
}

// === PGN 130310 - Environmental Parameters ===
func decodePGN130310(data []byte) (map[string]interface{}, error) {
	if len(data) < 12 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	airTempRaw := u16le(data, 4)
	waterTempRaw := u16le(data, 6)
	humidityRaw := u16le(data, 8)
	pressureRaw := u16le(data, 10)

	result["sid"] = sid

	if airTempRaw != 0xFFFF {
		result["air_temperature_c"] = float64(airTempRaw)*0.01 - 273.15
	}

	if waterTempRaw != 0xFFFF {
		result["water_temperature_c"] = float64(waterTempRaw)*0.01 - 273.15
	}

	if humidityRaw != 0xFFFF {
		result["relative_humidity_pct"] = float64(humidityRaw) * 0.004
	}

	if pressureRaw != 0xFFFF {
		result["atmospheric_pressure_hpa"] = float64(pressureRaw) * 0.1
	}

	return result, nil
}

// === PGN 130312 - Temperature ===
func decodePGN130312(data []byte) (map[string]interface{}, error) {
	if len(data) < 6 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	instance := u8(data, 1)
	source := u8(data, 2)
	actualTempRaw := u16le(data, 3)

	result["sid"] = sid
	result["temperature_instance"] = instance
	result["temperature_source"] = source

	if actualTempRaw != 0xFFFF {
		result["actual_temperature_c"] = float64(actualTempRaw)*0.01 - 273.15
	}

	if len(data) >= 7 {
		setTempRaw := u16le(data, 5)
		if setTempRaw != 0xFFFF {
			result["set_temperature_c"] = float64(setTempRaw)*0.01 - 273.15
		}
	}

	return result, nil
}

// === PGN 130313 - Humidity ===
func decodePGN130313(data []byte) (map[string]interface{}, error) {
	if len(data) < 6 {
		return nil, nil
	}

	result := make(map[string]interface{})
	sid := u8(data, 0)
	instance := u8(data, 1)
	source := u8(data, 2)
	actualHumidityRaw := u16le(data, 3)

	result["sid"] = sid
	result["humidity_instance"] = instance
	result["humidity_source"] = source

	if actualHumidityRaw != 0xFFFF {
		result["actual_humidity_pct"] = float64(actualHumidityRaw) * 0.004
	}

	return result, nil
}