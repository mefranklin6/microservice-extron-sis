package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mefranklin6/microservice-framework/framework" // Change after PR#3 for Dartmouth
)

var errorResponsesMap = map[string]string{
	"E10": "Unrecognized command",
	"E12": "Invalid port number",
	"E13": "Invalid paramater (number is out of range)",
	"E14": "Not valid for this configuration",
	"E17": "Invalid command for signal type",
	"E18": "System timed out",
	"E22": "Busy",
	"E24": "Privilege violation",
	"E28": "Bad name or file not found",
}

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
	"viewglobalmute":       "B\r", // non-matrix

	"viewinputname":         "\x1B%sNI\r",    // arg1: input name
	"queryhdcpinputstatus":  "\x1BI%sHDCP\r", // arg1: input name
	"queryhdcpoutputstatus": "\x1BO%sHDCP\r", // arg1: output name
}

var internalGetCmdMap = map[string]string{
	"viewvideosignalpresence": "\x1B0LS\r", // non-matrix
	"viewallinputconnections": "0LS\r",     // matrix
	"viewvideoinput":          "&\r",       // non-matrix
	"viewaudioinput":          "$\r",       // non-matrix
	"viewcurrentinput":        "!\r",       // non-matrix
	"viewloopoutinput":        "\x1BLOUT\r",
	"viewallvideoties":        "\x1B0*1*1VC\r", // matrix
	"viewallaudioties":        "\x1B0*1*2VC\r", // matrix
	"readvideooutputtie":      "%s%%\r",        // arg1: output name, matrix
	"readaudiooutputtie":      "%s$\r",         // arg1: output name, matrix
	"viewmutestatus":          "%s*B\r",        // non-matrix
	"viewoutputvideomutes":    "\x1BVM\r",      // matrix
}

// These can be called as endpoints but may not be part of OpenAV spec
var publicSetCmdEndpoints = map[string]string{
	"lockallfrontpanelfunctions":      "1X\r",
	"lockadvancedfrontpanelfunctions": "2X\r",
	"unlockallfrontpanelfunctions":    "0X\r",
}

var internalSetCmdMap = map[string]string{
	"globalvideomute":        "1*B\r",
	"globalvideoandsyncmute": "2*B\r",
	"globalvideounmute":      "0*B\r",

	"audioandvideoroute": "%s!\r",        // arg1: input name, non-matrix
	"videoroute":         "%s&\r",        // arg1: input name, non-matrix
	"audioroute":         "%s$\r",        // arg1: input name, non-matrix
	"setloopoutinput":    "\x1B%sLOUT\r", // arg1: input name

	"tieaudioandvideoroute": "%s*%s!\r", // arg1: input name | arg2: output name, matrix
	"tievideoroute":         "%s*%s%\r", // arg1: input name | arg2: output name, matrix
	"tieaudioroute":         "%s*%s$\r", // arg1: input name | arg2: output name, matrix

	"mutevideooutput":   "%s*1B\r", // arg1: output name
	"mutevideoandsync":  "%s*2B\r", // arg1: output name
	"unmutevideooutput": "%s*0B\r", // arg1: output name
}

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
			// 1. Copywright message
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

// Call this function before trying to write to the socket
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

// Checks if the device returned an error code.  If it did, return a formatted error message.
func formatDeviceErrMessage(socketKey string, resp string) string {
	function := "formatDeviceErrMessage"

	if errorMessage, exists := errorResponsesMap[resp]; exists { // known error
		errMsg := fmt.Sprintf("%s - I9WuD - device returned error: %s: %s", function, resp, errorMessage)
		return errMsg
	} else if strings.HasPrefix(resp, "E") && len(resp) == 3 { // unknown error
		errMsg := function + " - Gnlz6 - Device returned unknown error code: " + resp
		return errMsg
	}
	return ""
}

func formatCommand(command string, arg1 string, arg2 string, arg3 string) string {
	function := "formatCommand"

	var cmd string
	switch strings.Count(command, "%s") {
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

func findDeviceType(socketKey string) (string, error) {
	function := "findDeviceType"

	if deviceType, exists := deviceTypes[socketKey]; exists {
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
		deviceType = "Matrix Swicher"

	case strings.Contains(resp, "scaling presentation switcher"):
		deviceType = "Scaler" // IN 16xx series

	case resp == "seemless presentation switcher":
		deviceType = "Scaler" // IN 18xx series

	case resp == "streaming media processor":
		deviceType = "Streaming Media" // ex: SMP3xx

	//case strings.Contains(resp, "????"):
	//	deviceType = "Switcher" // non-scaling switchers, often older or low-end models

	default:
		deviceType = "unknown"
	}

	deviceTypes[socketKey] = deviceType

	return deviceType, nil
}

// MAIN FUNCTIONS

func getVideoRoute(socketKey string, output string) (string, error) {
	function := "getVideoRoute"

	value := `"unknown"`
	err := error(nil)
	maxRetries := 2
	for maxRetries > 0 {
		value, err = getVideoRouteDo(socketKey, output)
		if value == `"unknown"` { // Something went wrong - perhaps try again
			framework.Log(function + " - mlduq - retrying operation")
			maxRetries--
			time.Sleep(1 * time.Second)
			if maxRetries == 0 {
				errMsg := fmt.Sprintf(function + "bmi8g - max retries reached")
				framework.AddToErrors(socketKey, errMsg)
			}
		} else { // Succeeded
			maxRetries = 0
		}
	}

	return value, err
}

func getVideoRouteDo(socketKey string, output string) (string, error) {
	function := "getVideoRouteDo"

	framework.Log(function + " - output: " + output)

	deviceType, err := findDeviceType(socketKey)
	if err != nil {
		errMsg := fmt.Sprintf(function+" - QnKnu3 - error finding device type: %s", err.Error())
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	cmdString := ""
	switch deviceType {
	case "Matrix Switcher":
		cmdString = formatCommand(internalGetCmdMap["readvideooutputtie"], output, "", "")
	case "Scaler":
		cmdString = internalGetCmdMap["viewvideoinput"]
	}

	if cmdString == "" {
		errMsg := fmt.Sprintf(function+" - xzk5wH - no command found for device type: %s", deviceType)
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	resp, err := sendBasicCommand(socketKey, cmdString)
	if err != nil {
		errMsg := fmt.Sprintf(function+" - cid6bw - error getting video route: %s", err.Error())
		return errMsg, errors.New(errMsg)
	}

	framework.Log(fmt.Sprintf("%s - %s - response: %s", function, socketKey, resp))

	// some non-matrix devices have leading zeroes in the response, remove them
	if len(resp) == 2 && resp[0] == '0' {
		resp = resp[1:]
	}

	return resp, nil
}

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
