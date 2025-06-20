package main

// Mappings //
var errorResponsesMap = map[string]string{
	"E01": "Invalid input number",
	"E06": "Invalid input during auto-input switching",
	"E10": "Invalid command",
	"E11": "Invalid preset number",
	"E12": "Invalid port number",
	"E13": "Invalid value",
	"E14": "Invalid command for this configuration",
	"E17": "Invalid command for signal type",
	"E18": "System timed out",
	"E22": "Busy",
	"E24": "Privilege violation",
	"E25": "Device not present",
	"E26": "Maximum number of connections exceeded",
	"E28": "Bad name or file not found",
	"E33": "Bad file type for logo",
}

// These can be called as endpoints but may not be part of OpenAV spec
var publicGetCmdEndpoints = map[string]string{
	"firmwareversion":      "Q\r", // is universal across all products
	"temperature":          "W20STAT\r",
	"partnumber":           "N\r",
	"modelname":            "I\r", // does not work on older scalers
	"modeldescription":     "2I\r",
	"systemstatus":         "S\r",
	"systemmemoryusage":    "3I\r",
	"videooutputmutes":     "\x1BVM\r",
	"viewlockstatus":       "X\r",
	"serialnumber":         "99I\r",
	"macaddress":           "98I\r",
	"ipaddress":            "\x1BCI\r",
	"openconnections":      "\x1BCC\r",
	"systemprocessorusage": "11I\r",
	"viewpowersavemode":    "\x1BPSAV\r",

	"viewglobalmute":        "B\r", // non-matrix
	"viewloopoutinput":      "\x1BLOUT\r",
	"viewinputname":         "\x1B%sNI\r",    // arg1: input name
	"queryhdcpinputstatus":  "\x1BI%sHDCP\r", // arg1: input name
	"queryhdcpoutputstatus": "\x1BO%sHDCP\r", // arg1: output name
}

// These can be called as endpoints but may not be part of OpenAV spec
var publicSetCmdEndpoints = map[string]string{
	"lockallfrontpanelfunctions":      "1X\r",
	"lockadvancedfrontpanelfunctions": "2X\r",
	"unlockallfrontpanelfunctions":    "0X\r",
}

// OpenAV spec get endpoint names with mappings for different device types
var internalGetCmdMap = map[string]map[string]string{
	"inputstatus": {
		"Matrix Switcher":        "0LS\r",
		"Scaler":                 "\x1B0LS\r",
		"Distribution Amplifier": "\x1BLS\r",
		"Switcher":               "\x1BLS\r",
	},
	"videoroute": {
		"Matrix Switcher": "%s%%\r", // arg1: output name
		"Scaler":          "&\r",
		"Switcher":        "!\r",
	},
	//"audioroute": {
	//	"Matrix Switcher": "%s$\r", // arg1: output name
	//	"Scaler":          "$\r",
	//},
	"audiomute": {
		"Matrix Switcher": "%s*B\r", // arg1: output name
		"Scaler":          "*B\r",
		"Switcher":        "\x1BAFMT\r",
	},
	"videomute": {
		"Matrix Switcher":        "\x1BVM\r",
		"Scaler":                 "B\r",
		"Switcher":               "B\r",
		"Distribution Amplifier": "B\r",
	},
	"matrixmute": {
		"Audio Processor": "\x1BM%sAU\r",
	},

	//"viewvideoinput":          "&\r",       // non-matrix
	//"viewcurrentinput":        "!\r",       // non-matrix

	//"viewallvideoties":        "\x1B0*1*1VC\r", // matrix
	//"viewallaudioties":        "\x1B0*1*2VC\r", // matrix

	//"readvideooutputtie":      "%s%%\r",        // arg1: output name, matrix

	//"viewmutestatus":          "%s*B\r",        // non-matrix

	//"viewoutputvideomutes":    "\x1BVM\r",      // matrix
}

// TODO
var internalSetCmdMap = map[string]map[string]string{
	"videoroute": {
		"Matrix Switcher": "%s*%s%%\r", // arg1: input name | arg2: output name
		"Scaler":          "%s&\r",     // arg1: input name
		"Switcher":        "%s!\r",     // arg1: input name
	},
	"audioandvideoroute": {
		"Matrix Switcher": "%s*%s!\r", // arg1: input name | arg2: output name
		"Scaler":          "%s!\r",    // arg1: input name
		"Switcher":        "%!\r",     // arg1: input name
	},
	"videomute": {
		"Matrix Switcher":        "%s*1B\r", // arg1: output name
		"Scaler":                 "%s*1B\r",
		"Switcher":               "1B\r",
		"Distribution Amplifier": "%s*1B\r",
	},
	"videounmute": {
		"Matrix Switcher":        "%s*0B\r", // arg1: output name
		"Scaler":                 "%s*0B\r",
		"Switcher":               "0B\r",
		"Distribution Amplifier": "%s*0B\r",
	},
	"videosyncmute": {
		"Matrix Switcher":        "%s*2B\r", // arg1: output name
		"Scaler":                 "%s*2B\r",
		"Switcher":               "2B\r",
		"Distribution Amplifier": "%s*2B\r",
	},
	"audiomute": {
		"Switcher": "\x1B1AFMT\r",
	},
	"audiounmute": {
		"Switcher": "\x1B0AFMT\r",
	},

	//"globalvideomute":        "1*B\r",
	//"globalvideoandsyncmute": "2*B\r",
	//"globalvideounmute":      "0*B\r",

	//"audioandvideoroute": "%s!\r",        // arg1: input name, non-matrix
	//"videoroute":         "%s&\r",        // arg1: input name, non-matrix
	//"audioroute":         "%s$\r",        // arg1: input name, non-matrix
	//"setloopoutinput":    "\x1B%sLOUT\r", // arg1: input name

	//"tieaudioandvideoroute": "%s*%s!\r", // arg1: input name | arg2: output name, matrix
	//"tieaudioroute":         "%s*%s$\r", // arg1: input name | arg2: output name, matrix

	//"mutevideooutput":   "%s*1B\r", // arg1: output name
	//"mutevideoandsync":  "%s*2B\r", // arg1: output name
	//"unmutevideooutput": "%s*0B\r", // arg1: output name
}

// Maps get endpoints to get functions so we can call them dynamically.
// Make sure all future endpoints are added here.
// function args are socketKey, endpoint, arg1, arg2, arg3
var getFunctionsMap = map[string]func(string, string, string, string, string) (string, error){
	"power":              notImplemented, // TODO
	"volume":             notImplemented, // TODO
	"videoroute":         getVideoRouteDo,
	"audioandvideoroute": notImplemented, // TODO
	"audiomute":          notImplemented, // TODO
	"videomute":          getVideoMuteDo,
	"audioandvideomute":  notImplemented, // TODO
	"inputstatus":        getInputStatusDo,
	"occupancystatus":    notImplemented, // TODO
	"matrixmute":         getMatrixMuteDo,
	"matrixvolume":       notImplemented, // TODO
	"setstate":           notImplemented, // TODO
}

// Maps set endpoints to set functions so we can call them dynamically.
// Make sure all future endpoints are added here.
// function args are socketKey, endpoint, arg1, arg2, arg3
var setFunctionsMap = map[string]func(string, string, string, string, string) (string, error){
	"power":              notImplemented, // TODO
	"volume":             notImplemented, // TODO
	"videoroute":         setVideoRouteDo,
	"audioandvideoroute": setAudioAndVideoRoute,
	"audiomute":          notImplemented, // TODO
	"videomute":          setVideoMuteDo,
	"videosyncmute":      setVideoSyncMuteDo,
	"audioandvideomute":  notImplemented, // TODO
	"matrixmute":         notImplemented, // TODO
	"matrixvolume":       notImplemented, // TODO
	"setstate":           notImplemented, // TODO
	"triggerstate":       notImplemented, // TODO
	"timedtriggerstate":  notImplemented, // TODO
}

// Input or Output Names : Index of where to find them in string returns (ex: video mute)
type videoIO_Map struct {
	inputs  map[string]int
	outputs map[string]int
}

var crossPoint84Map = videoIO_Map{
	inputs: map[string]int{
		"1": 0,
		"2": 1,
		"3": 2,
		"4": 3,
		"5": 4,
		"6": 5,
		"7": 6,
		"8": 7,
	},
	outputs: map[string]int{
		"1":  0,
		"2":  1,
		"3A": 2,
		"3B": 3,
		"4A": 4,
		"4B": 5,
	},
}

var crossPoint86Map = videoIO_Map{
	inputs: map[string]int{
		"1": 0,
		"2": 1,
		"3": 2,
		"4": 3,
		"5": 4,
		"6": 5,
		"7": 6,
		"8": 7,
	},
	outputs: map[string]int{
		"1":  0,
		"2":  1,
		"3A": 2,
		"3B": 3,
		"4A": 4,
		"4B": 5,
		"5":  6,
		"6":  7,
	},
}

var crossPoint108Map = videoIO_Map{
	inputs: map[string]int{
		"1":  0,
		"2":  1,
		"3":  2,
		"4":  3,
		"5":  4,
		"6":  5,
		"7":  6,
		"8":  7,
		"9":  8,
		"10": 9,
	},
	outputs: map[string]int{
		"1":  0,
		"2":  1,
		"3":  2,
		"4":  3,
		"5A": 4,
		"5B": 5,
		"6A": 6,
		"6B": 7,
		"7":  8,
		"8":  9,
	},
}

// LoopOut is handled as a special case
var in180xMap = videoIO_Map{
	inputs: map[string]int{
		"1": 0,
		"2": 1,
		"3": 2,
		"4": 3,
		"5": 4,
		"6": 5,
		"7": 6,
		"8": 7,
	},
	outputs: map[string]int{
		"1A": 0,
		"1B": 1,
		// LoopOut is not in the map, it is handled separately
	},
}

// DMP Maths

type TableKey string

const (
	// Output-mix matrices (Tables 3-5)
	MicToOut   TableKey = "MicToOut"   // Table 3 – Mic/Line → Line Out
	VRetToOut  TableKey = "VRetToOut"  // Table 4 – Virtual Return → Line Out
	EXPInToOut TableKey = "EXPInToOut" // Table 5 – EXP In → Line Out

	// Virtual-send matrices (Tables 6-8)
	MicToSend   TableKey = "MicToSend"   // Table 6 – Mic/Line → Virtual Send
	VRetToSend  TableKey = "VRetToSend"  // Table 7 – Virtual Return → Virtual Send
	EXPInToSend TableKey = "EXPInToSend" // Table 8 – EXP In → Virtual Send

	// EXP-output matrices (Tables 9-10)
	MicToEXPOut  TableKey = "MicToEXPOut"  // Table 9 – Mic/Line → EXP Out
	VRetToEXPOut TableKey = "VRetToEXPOut" // Table 10 – Virtual Return → EXP Out
)

// Base addresses for DMP matrix tables
var dmpBaseAddr = map[TableKey]int{
	MicToOut:     20000, // Table 3
	VRetToOut:    21300, // Table 4
	EXPInToOut:   22100, // Table 5
	MicToSend:    20009, // Table 6
	VRetToSend:   21309, // Table 7
	EXPInToSend:  22109, // Table 8
	MicToEXPOut:  20018, // Table 9
	VRetToEXPOut: 21317, // Table 10
}
