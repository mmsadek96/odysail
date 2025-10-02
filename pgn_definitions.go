package nmea

// MeasurementMap classifies PGNs by measurement type
var MeasurementMap = map[int]string{
	// Navigation & Position
	129025: "position",
	129026: "navigation",
	129029: "position",
	127250: "heading",
	127251: "navigation",
	128259: "navigation",
	128267: "navigation",
	128275: "log",
	129284: "navigation",
	129285: "navigation",
	129540: "gnss",

	// Wind & Weather
	130306: "wind",
	130310: "environment",
	130311: "environment",
	130312: "environment",
	130313: "environment",
	130314: "environment",

	// Engine & Propulsion
	127488: "engine",
	127489: "engine",
	127493: "transmission",
	127497: "engine",
	127498: "engine",
	127500: "dc_power",
	127501: "dc_power",
	127502: "dc_power",
	127503: "ac_power",
	127504: "ac_power",
	127505: "dc_power",
	127506: "dc_power",
	127507: "dc_power",
	127508: "dc_power",
	127509: "dc_power",

	// Attitude (IMU - Critical for BoomSense)
	127257: "attitude",
	127252: "attitude",

	// AIS
	129038: "ais",
	129039: "ais",
	129040: "ais",
	129793: "ais",
	129794: "ais",
	129798: "ais",
	129802: "ais",
	129809: "ais",
	129810: "ais",

	// System
	126992: "system",
	126993: "system",
	126996: "system",
	126998: "system",
	127245: "autopilot",
	127237: "autopilot",
	127258: "autopilot",
	126208: "system",

	// Small Craft
	130576: "craft_status",
	130577: "craft_status",

	// Proprietary
	126720: "proprietary",
	130822: "proprietary",
}

// PGNNames provides human-readable names
var PGNNames = map[int]string{
	129025: "Position Rapid Update",
	129026: "COG & SOG Rapid Update",
	129029: "GNSS Position Data",
	127250: "Vessel Heading",
	127251: "Rate of Turn",
	127257: "Attitude",
	127252: "Heave",
	128259: "Speed Water Referenced",
	128267: "Water Depth",
	128275: "Distance Log",
	129284: "Navigation Data",
	129285: "Route/WP Information",
	129540: "GNSS Satellites in View",
	130306: "Wind Data",
	130310: "Environmental Parameters",
	130311: "Environmental Parameters",
	130312: "Temperature",
	130313: "Humidity",
	130314: "Actual Pressure",
	127488: "Engine Parameters Rapid",
	127489: "Engine Parameters Dynamic",
	127493: "Transmission Parameters",
	127497: "Trip Parameters Engine",
	127498: "Engine Parameters Static",
	127500: "Load Controller State",
	127501: "Binary Switch Bank Status",
	127502: "Switch Bank Control",
	127503: "AC Input Status",
	127504: "AC Output Status",
	127505: "Fluid Level",
	127506: "DC Detailed Status",
	127507: "Charger Status",
	127508: "Battery Status",
	127509: "Inverter Status",
	129038: "AIS Class A Position",
	129039: "AIS Class B Position",
	129040: "AIS Class B Extended Position",
	129793: "AIS UTC Date/Time",
	129794: "AIS Class A Static Data",
	129798: "AIS SAR Aircraft Position",
	129802: "AIS Safety Broadcast",
	129809: "AIS Class B Static A",
	129810: "AIS Class B Static B",
	126992: "System Time",
	126993: "Heartbeat",
	126996: "Product Information",
	126998: "Configuration Information",
	127245: "Rudder",
	127237: "Heading/Track Control",
	127258: "Magnetic Variation",
	126208: "Group Function",
	130576: "Small Craft Status",
	130577: "Direction Data",
	126720: "Proprietary",
	130822: "Proprietary Fast",
}

// GetMeasurementType returns the measurement classification for a PGN
func GetMeasurementType(pgn int) string {
	if m, ok := MeasurementMap[pgn]; ok {
		return m
	}
	return "nmea_general"
}

// GetPGNName returns the human-readable name for a PGN
func GetPGNName(pgn int) string {
	if name, ok := PGNNames[pgn]; ok {
		return name
	}
	return "Unknown"
}

// PGNFromParts calculates PGN from CAN ID components
func PGNFromParts(dp, pf, ps uint8) int {
	base := (int(dp&0x01) << 16) | (int(pf&0xFF) << 8)
	if pf < 240 {
		return base
	}
	return base | int(ps&0xFF)
}