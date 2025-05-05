package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mefranklin6/microservice-framework/framework" // Change after PR#3 for Dartmouth
)

// Mappings //

var errorResponsesMap = map[string]string{
	"E10": "Unrecognized command",
	"E12": "Invalid port number",
	"E13": "Invalid parameter (number is out of range)",
	"E14": "Not valid for this configuration",
	"E17": "Invalid command for signal type",
	"E18": "System timed out",
	"E22": "Busy",
	"E24": "Privilege violation",
	"E28": "Bad name or file not found",
}

var deviceTypes = make(map[string]string) // socketKey -> deviceType

// These can be called as endpoints but may not be part of OpenAV spec
var publicGetCmdEndpoints = map[string]string{
	"firmwareversion":      "Q\r", // is universal across all products
	"temperature":          "W20STAT\r",
	"partnumber":           "N\r",
	"modelname":            "I\r",
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
	},
	"videoroute": {
		"Matrix Switcher": "%s%%\r", // arg1: output name
		"Scaler":          "&\r",
	},
	//"audioroute": {
	//	"Matrix Switcher": "%s$\r", // arg1: output name
	//	"Scaler":          "$\r",
	//},
	"audiomute": {
		"Matrix Switcher": "%s*B\r", // arg1: output name
		"Scaler":          "*B\r",
	},
	"videomute": {
		"Matrix Switcher":        "\x1BVM\r",
		"Scaler":                 "B\r",
		"Switcher":               "B\r",
		"Distribution Amplifier": "B\r",
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
	},
	"audioandvideoroute": {
		"Matrix Switcher": "%s*%s!\r", // arg1: input name | arg2: output name
		"Scaler":          "%s!\r",    // arg1: input name
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
	"matrixmute":         notImplemented, // TODO
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
	"audioandvideoroute": setAudioAndVideoRoute, // TODO
	"audiomute":          notImplemented,        // TODO
	"videomute":          notImplemented,        // TODO
	"videosyncmute":      notImplemented,        // TODO
	"audioandvideomute":  notImplemented,        // TODO
	"matrixmute":         notImplemented,        // TODO
	"matrixvolume":       notImplemented,        // TODO
	"setstate":           notImplemented,        // TODO
	"triggerstate":       notImplemented,        // TODO
	"timedtriggerstate":  notImplemented,        // TODO
}

// Main Functions //

// Get functions //
func getVideoRouteDo(socketKey string, endpoint string, output string, _ string, _ string) (string, error) {
	function := "getVideoRouteDo"

	resp, err := deviceTypeDependantCommand(socketKey, "videoroute", "GET", output, "", "")
	if err != nil {
		errMsg := function + "- error getting video route: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// some non-matrix devices have leading zeroes in the response, remove them.
	// remember the response is wrapped in quotes
	if len(resp) == 4 && resp[1] == '0' {
		resp = `"` + resp[2:]
	}
	return resp, nil
}

func getInputStatusDo(socketKey string, endpoint string, input string, _ string, _ string) (string, error) {
	function := "getInputStatusDo"

	resp, err := deviceTypeDependantCommand(socketKey, "inputstatus", "GET", input, "", "")
	if err != nil {
		errMsg := function + "- error getting input status: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// matrix will return string of 1 or 0 for all inputs it supports
	// scaler will do the same but with "*" between inputs
	// DA will return "input*loopout output1 output2..."   ex: "1*0 0 1 0 0"

	// handle Distribution Amplifier (one input only)
	if strings.Count(resp, "*") == 1 && len(resp) > 1 && (resp[1] == '1' || resp[1] == '0') {
		resp = `"` + resp[1:]
		if resp == "1" {
			return "true", nil
		} else if resp == "0" {
			return "false", nil
		} else {
			errMsg := function + " - invalid response for DA input status: " + resp
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
	}

	// Remove the matrix formatting
	resp = strings.ReplaceAll(resp, `*`, ``)

	// cast the input string to an integer
	inputNum, err := strconv.Atoi(input)
	if err != nil {
		errMsg := function + " - invalid input number: " + input
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Check if index is in bounds
	if inputNum < 0 || inputNum >= len(resp) {
		errMsg := function + " - input number out of range: " + input
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Extract the single character
	result := string(resp[inputNum])

	// 'cast' result to 'bool' (still a string)
	if result == "1" {
		result = "true"
	} else if result == "0" {
		result = "false"
	}

	return result, nil
}

func getVideoMuteDo(socketKey string, endpoint string, output string, _ string, _ string) (string, error) {
	function := "getVideoMuteDo"

	resp, err := deviceTypeDependantCommand(socketKey, "videomute", "GET", "", "", "")
	if err != nil {
		errMsg := function + "- error getting video mute status: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// all return strings contain "0" (not muted), "1" (muted with sync) or 2 (sync mute) for each output

	// DA returns string with single space between the characters.  First character is input.
	// Matrix with "A" or "B" ex CP84: "0 0 0 0 0 0" which is 1,2,3A,3B,4A,4B
	// Scaler with loopout (IN 1808) : "0 0 0", which is 1A,1B,LoopOut
	// Scaler with mirrored but individually-mute-controlled outs (IN 1804) : "0 0"
	// Scaler (IN 16xx) with standard mirrored outputs, just 0,1, or 2

	resp = strings.ReplaceAll(resp, " ", "")
	resp = strings.ReplaceAll(resp, `"`, ``)

	deviceType := deviceTypes[socketKey]

	// Simple device, only one character reply
	// Could be IN 16xx or a switcher
	if resp == "0" {
		return "false", nil
	} else if resp == "1" {
		return "true", nil
	} else if resp == "2" { // sync mute
		return "true", nil
	}

	// Query is for loop out, could be IN1808
	if output == "LoopOut" {
		framework.Log(fmt.Sprintf("%s - %s - LoopOut response: %s", function, socketKey, resp))
		result := string(resp[2])
		switch result {
		case "0":
			return "false", nil
		case "1":
			return "true", nil
		case "2":
			return "true", nil
		default:
			errMsg := function + " - invalid loopout response: " + resp
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
	}

	// DA: the first character is the input
	if deviceType == "Distribution Amplifier" {
		outputInt, err := strconv.Atoi(output)
		if err != nil {
			errMsg := function + " - invalid output number: " + output
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
	result := string(resp[outputInt+1]) // +1 to skip the input
		switch result {
		case "0":
			return "false", nil
		case "1":
			return "true", nil
		case "2":
			return "true", nil
	}

	// Maps: {Input Name : String index of where to find the output in resp}
	// Note: So far these are all just both just unique and similar enough,
	// ex: only the 108 has "6A" and all devices that have a "3B" index on 4.

	// Make sure to call the correct output;
	// Calling "4" when you meant "4A" on a CP84 would result in an incorrect response (CP108 index)

	// ...this may change in the future as more devices are added and may require a re-design.

	var crossPoint84Map = map[string]int{
		"1":  0,
		"2":  1,
		"3A": 2,
		"3B": 3,
		"4A": 4,
		"4B": 5,
	}

	var crossPoint86Map = map[string]int{
		"1":  0,
		"2":  1,
		"3A": 2,
		"3B": 3,
		"4A": 4,
		"4B": 5,
		"5":  6,
		"6":  7,
	}

	var crossPoint108Map = map[string]int{
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
	}

	// LoopOut is already handled above
	var in180xMap = map[string]int{
		"1A": 0,
		"1B": 1,
	}

	// Check if output is in one of the device maps

	var index int
	found := false

	// Check Crosspoint Maps
	if deviceType == "Matrix Switcher" {
		if idx, ok := crossPoint84Map[output]; ok {
			index = idx
			found = true
		} else if idx, ok := crossPoint86Map[output]; ok {
			index = idx
			found = true
		} else if idx, ok := crossPoint108Map[output]; ok {
			index = idx
			found = true
		}
	}

	// Check Scaler Maps
	if deviceType == "Scaler" {
		if idx, ok := in180xMap[output]; ok {
			index = idx
			found = true
		}
	}

	if !found {
		errMsg := function + " - output not found in any device map: " + output
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	framework.Log(fmt.Sprintf("%s - %s - output: %s, is at index: %d of %s", function, socketKey, output, index, resp))

	result := string(resp[index])

	framework.Log(fmt.Sprintf("%s - %s - result: %s", function, socketKey, result))

	switch result {
	case "0":
		return "false", nil
	case "1":
		return "true", nil
	case "2":
		return "true", nil
	default:
		errMsg := function + " - invalid response for output: " + output + ": " + resp
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
}

// Set functions //

func setVideoRouteDo(socketKey string, endpoint string, input string, output string, _ string) (string, error) {
	function := "setVideoRouteDo"

	resp, err := deviceTypeDependantCommand(socketKey, "videoroute", "SET", input, output, "")
	if err != nil {
		errMsg := function + "- error setting video route: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	// Good response example is "Out4 In6 Vid" for a matrix switcher "In6 RGB" for scaler
	// Any errors will have been formatted in formatDeviceErrMessage

	switch {
	case strings.Contains(resp, "error"):
		return resp, errors.New(resp) // device returned an error code
	case strings.Contains(resp, "In") && strings.Contains(resp, input):
		return "ok", nil
	default:
		return "unknown response: " + resp, nil
	}
}

func setAudioAndVideoRoute(socketKey string, endpoint string, input string, output string, _ string) (string, error) {
	function := "setAudioAndVideoRoute"

	resp, err := deviceTypeDependantCommand(socketKey, "audioandvideoroute", "SET", input, output, "")
	if err != nil {
		errMsg := function + "- error setting audio and video route: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Matrix good response: "Out4 In2 All"
	// Scaler good response: "In02 All"
	switch {
	case strings.Contains(resp, "error"):
		return resp, errors.New(resp) // device returned an error code
	case strings.Contains(resp, "In") && strings.Contains(resp, input) && strings.Contains(resp, "All"):
		return "ok", nil
	default:
		return "unknown response: " + resp, nil
	}
}

// Helper functions //

// Placeholder for not implemented functions
func notImplemented(socketKey string, endpoint string, _ string, _ string, _ string) (string, error) {
	function := "notImplemented"

	errMsg := fmt.Sprintf("%s - %s - endpoint '%s' is not implemented", function, socketKey, endpoint)
	framework.AddToErrors(socketKey, errMsg)
	return "", errors.New(errMsg)
}

// Internal
func loginNegotiation(socketKey string) (success bool) {
	function := "loginNegotiation"

	// Get password. Extron Telnet connection assumes 'admin' as username
	password := "" // device expects empty string if no password is set
	if strings.Count(socketKey, "@") == 1 {
		credentials := strings.Split(socketKey, "@")[0]
		if strings.Count(credentials, ":") == 1 {
			password = strings.Split(credentials, ":")[1]
		}
	}

	count := 0
	// Breaks if the negotiations go over 7 rounds to avoid an infinite loop.
	for count < 7 {
		count += 1
		negotiationResp := framework.ReadLineFromSocket(socketKey)
		framework.Log("Printing Negotiation from Extron SIS device: " + negotiationResp)

		if password != "" {
			if strings.Contains(negotiationResp, "Password:") {
				sent := framework.WriteLineToSocket(socketKey, password+"\r")
				if !sent {
					errMsg := function + " - k4j5d3m - Failed to send password"
					framework.AddToErrors(socketKey, errMsg)
					return false
				}
			}
			// Check for successful login
			if strings.HasPrefix(negotiationResp, "Login") {
				framework.Log("Login successful. Command line prompt is " + negotiationResp)
				return true
			}
		} else {
			// TODO: Implement unauthenticated login
			// If no password is set, device will follow this pattern:
			// 1. Copyright message
			// 2. Current date
			// 3. Empty line.  Also sometimes expects a delay before first command
			framework.AddToErrors(socketKey, function+" - k4j5d3m - unauthenticated login not implemented yet.  Please set a password.")
			return true
		}
	}

	errMsg := function + " - mrk42 - Stopped negotiation loop after 7 rounds to avoid infinite loop."
	framework.AddToErrors(socketKey, errMsg)

	return false
}

// Internal function that's called before writing to the socket
func ensureActiveConnection(socketKey string) error {
	function := "ensureActiveConnection"

	connected := framework.CheckConnectionsMapExists(socketKey)
	if connected == false {
		if framework.UseTelnet {
			negotiation := loginNegotiation(socketKey)
			if negotiation == false {
				errMsg := fmt.Sprintf(function + " - h3boid - error logging in")
				framework.AddToErrors(socketKey, errMsg)
				return errors.New(errMsg)
			}
		} else {
			return nil // assume serial connection
		}
	}
	return nil // Connection map already in framework
}

// Internal: Checks if the device returned an error code.  If it did, return a formatted error message.
func formatDeviceErrMessage(socketKey string, resp string) string {
	function := "formatDeviceErrMessage"

	// make sure to always include "error" in the response if there's an error
	if errorMessage, exists := errorResponsesMap[resp]; exists { // known error
		errMsg := fmt.Sprintf("%s - I9WuD - device returned error: %s: %s", function, resp, errorMessage)
		return errMsg
	} else if strings.HasPrefix(resp, "E") && len(resp) == 3 { // unknown error
		errMsg := function + " - Gnlz6 - Device returned unknown error code: " + resp
		return errMsg
	}
	return ""
}

// Internal: Formats the command string with the provided arguments.
func formatCommand(command string, arg1 string, arg2 string, arg3 string) string {
	function := "formatCommand"

	var cmd string

	framework.Log(function + " - Formatting command: " + command)
	framework.Log(function + " - Arguments: " + arg1 + ", " + arg2 + ", " + arg3)

	// Count the number of non-empty arguments
	verbCount := strings.Count(command, "%s")

	switch verbCount {
	case 3:
		cmd = fmt.Sprintf(command, arg1, arg2, arg3)
	case 2:
		cmd = fmt.Sprintf(command, arg1, arg2)
	case 1:
		cmd = fmt.Sprintf(command, arg1)
	default:
		cmd = command
	}

	framework.Log(function + " - Formatted command: " + cmd)
	return cmd
}

// Internal: returns the device type from a package-level cache or queries the device.
func findDeviceType(socketKey string) (string, error) {
	function := "findDeviceType"

	if deviceType, exists := deviceTypes[socketKey]; exists {
		framework.Log(fmt.Sprintf("%s - %s - Device type found in cache: %s", function, socketKey, deviceType))
		return deviceType, nil // cache hit
	}

	// Haven't heard from device yet, send a query
	cmdString := publicGetCmdEndpoints["modeldescription"]
	resp, err := sendBasicCommand(socketKey, cmdString)
	if err != nil {
		errMsg := fmt.Sprintf(function+" - jrBaq3 - error getting device type: %s", err.Error())
		return "", errors.New(errMsg)
	}

	logStr := fmt.Sprintf("%s - %s - Device type response: %s", function, socketKey, resp)
	framework.Log(logStr)

	deviceType := "unknown"

	resp = strings.ToLower(resp)

	// TODO: commented out items
	switch {
	case strings.Contains(resp, "dmp") || strings.Contains(resp, "digital audio"):
		deviceType = "Audio Processor"

	//case strings.Contains(resp, "TODO:"):
	//	deviceType = "Collaboration Systems" //ex: ShareLink

	case strings.Contains(resp, "matrix") && !strings.Contains(resp, "audio"):
		deviceType = "Matrix Switcher"

	case strings.Contains(resp, "scaling presentation switcher"):
		deviceType = "Scaler" // IN 16xx series

	case resp == "seemless presentation switcher":
		deviceType = "Scaler" // IN 18xx series

	case resp == "streaming media processor":
		deviceType = "Streaming Media" // ex: SMP3xx

	//case strings.Contains(resp, "????"):
	//	deviceType = "Switcher" // non-scaling switchers, often older or low-end models

	case strings.Contains(resp, "distribution amplifier"):
		deviceType = "Distribution Amplifier"

	default:
		deviceType = "unknown"
	}

	deviceTypes[socketKey] = deviceType
	framework.Log(fmt.Sprintf("%s - %s - Device type determined: %s", function, socketKey, deviceType))

	return deviceType, nil
}

// Main function that handles device type dependent commands
func deviceTypeDependantCommand(socketKey string, endpoint string, method string, arg1 string, arg2 string, arg3 string) (string, error) {
	function := "deviceTypeDependantCommand"

	deviceType, err := findDeviceType(socketKey)
	if err != nil {
		errMsg := fmt.Sprintf(function+" - l6ehb - error finding device type: %s", err.Error())
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	cmdString := ""
	var cmdMap map[string]map[string]string
	if method == "GET" {
		cmdMap = internalGetCmdMap
	} else if method == "SET" {
		cmdMap = internalSetCmdMap
	} else {
		errMsg := fmt.Sprintf(function+" - 8deoi - invalid method: %s", method)
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	cmdString = formatCommand(cmdMap[endpoint][deviceType], arg1, arg2, arg3)

	if cmdString == "" {
		errMsg := fmt.Sprintf(function+" - 8deoi - no command found for device type: %s", deviceType)
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	resp, err := sendBasicCommand(socketKey, cmdString)
	if err != nil {
		errMsg := fmt.Sprintf(function+" - cid6bw - error getting endpoint: %s: %s", endpoint, err.Error())
		return errMsg, errors.New(errMsg)
	}

	framework.Log(fmt.Sprintf("%s - %s - %s response: %s", function, socketKey, endpoint, resp))
	return resp, nil
}

// entry point for special endpoints that require their own get function
func specialEndpointGet(socketKey string, endpoint string, arg1 string, arg2 string, arg3 string) (string, error) {
	function := "specialEndpointGet"

	value := `"unknown"`
	err := error(nil)

	if fn, exists := getFunctionsMap[endpoint]; exists {
		value, err = fn(socketKey, endpoint, arg1, arg2, arg3)
	} else {
		errMsg := fmt.Sprintf(function+" - 7s5ce - no special get function found for endpoint: %s", endpoint)
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	return value, err
}

// entry point for special endpoints that require their own set function
func specialEndpointSet(socketKey string, endpoint string, arg1 string, arg2 string, arg3 string) (string, error) {
	function := "specialEndpointSet"

	value := `"unknown"`
	err := error(nil)

	if fn, exists := setFunctionsMap[endpoint]; exists {
		value, err = fn(socketKey, endpoint, arg1, arg2, arg3)
	} else {
		errMsg := fmt.Sprintf(function+" - kh6na - no special set function found for endpoint: %s", endpoint)
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	return value, err
}

// Lower level main send command.
func sendBasicCommand(socketKey string, cmdString string) (string, error) {
	function := "sendBasicCommand"

	framework.Log(function + " - cmdString: " + cmdString)

	value := `"unknown"`
	err := error(nil)
	maxRetries := 2
	for maxRetries > 0 {
		value, err = sendBasicCommandDo(socketKey, cmdString)
		if value == `"unknown"` { // Something went wrong - perhaps try again
			framework.Log(function + " - fq3sdvc - retrying operation")
			maxRetries--
			time.Sleep(1 * time.Second)
			if maxRetries == 0 {
				errMsg := fmt.Sprintf(function + " - f839dk4 - max retries reached")
				framework.AddToErrors(socketKey, errMsg)
			}
		} else { // Succeeded
			maxRetries = 0
		}
	}

	return value, err
}

// Internal
func sendBasicCommandDo(socketKey string, cmdString string) (string, error) {
	function := "sendBasicCommandDo"

	err := ensureActiveConnection(socketKey)
	if err != nil {
		framework.AddToErrors(socketKey, err.Error())
		return "", err
	}

	sent := framework.WriteLineToSocket(socketKey, cmdString)
	if sent != true {
		errMsg := fmt.Sprintf(function + " - i5kcfoe - error sending command")
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	resp := framework.ReadLineFromSocket(socketKey)

	deviceErrMsg := formatDeviceErrMessage(socketKey, resp)
	if deviceErrMsg != "" {
		framework.AddToErrors(socketKey, deviceErrMsg)
		resp = deviceErrMsg // Return the error message as the response
	}

	resp = strings.TrimPrefix(resp, `"`)
	resp = strings.TrimSuffix(resp, `"`)

	return `"` + resp + `"`, nil
}
