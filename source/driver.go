package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mefranklin6/microservice-framework/framework" // TODO: Change after PR#5 to Dartmouth-OpenAV
)

// Package-level variables
var deviceTypes = make(map[string]string)              // socketKey -> deviceType
var deviceModels = make(map[string]string)             // socketKey -> modeldescription
var keepAlivePollRoutines = make(map[string]chan bool) // socketKey -> stop channel
var keepAlivePollRoutinesMutex sync.Mutex
var txRxMutexes sync.Map // socketKey -> *sync.Mutex

///////////////////////////////////////////////////////////////////////////////
// Main functions //
///////////////////////////////////////////////////////////////////////////////

// Get functions //

// This endpoint has limited functionality within the Extron ecosystem.
// It is only used for group volume control of scalers.
// For dedicated DSP units and matrix switchers, you'll want to use "matrixvolume" instead.
// Note: IN1804 breaks all the patterns and is not supported yet.
func getVolumeDo(socketKey string, endpoint string, name string, _ string, _ string) (string, error) {
	function := "getVolumeDo"

	model, err := findModelName(socketKey)
	if err != nil {
		modelErr := function + " - can not find model for: " + socketKey
		framework.AddToErrors(socketKey, modelErr)
		return modelErr, errors.New(modelErr)
	}

	var oid string // mix point number
	ok := false

	// Check if model is supported
	switch {
	case strings.Contains(model, "160") && strings.Contains(model, "IN"): // IN 160x series
		oid, ok = in160xGroupAudioVolumeMap[name] // Only group voloumes on 160x series (firmware bug)
	default:
		notImpMsg := function + "Model: " + model + " is not implemented or does not support 'volume'"
		framework.AddToErrors(socketKey, notImpMsg)
		return notImpMsg, errors.New(notImpMsg)
	}

	// Check if we have the channel name+oid mapping for the model
	if !ok {
		errMsg := function + "Can't find OID for: " + name + " on model: " + model
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Try sending the command
	resp, err := deviceTypeDependantCommand(socketKey, "volume", "GET", oid, "", "")
	if err != nil {
		errMsg := function + " - error getting volume " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Convert return in tenths of DB to percent
	percent, err := newUnTransformVolume(resp)
	if err != nil {
		errMsg := function + " - error converting device volume to percent: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	return percent, nil
}

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
	resp = strings.ReplaceAll(resp, `"`, ``)

	// matrix will return string of 1 or 0 for all inputs it supports
	// scaler will do the same but with "*" between inputs
	// DA will return "input*loopout output1 output2..."   ex: "1*0 0 1 0 0"
	// switcher will return backwards DA, ex: "1 0 1 0*0" (output is last)

	// handle Distribution Amplifier (one input only)
	if strings.Count(resp, "*") == 1 && len(resp) >= 2 && string(resp[1]) == "*" && (string(resp[0]) == "1" || string(resp[0]) == "0") {
		input := string(resp[0])
		if input == "1" {
			return "true", nil
		} else if input == "0" {
			return "false", nil
		} else {
			errMsg := function + " - invalid response for DA input status: " + resp
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
	}

	// Switcher: "*" should be second to last character)
	if strings.Count(resp, "*") == 1 {
		resp = strings.ReplaceAll(resp, "\r", "")
		starIndx := strings.Index(resp, "*")
		if starIndx == len(resp)-2 {
			resp = resp[:len(resp)-2]                // remove the star and output
			resp = strings.ReplaceAll(resp, " ", "") // remove spaces
		} else {
			errMsg := function + " - invalid response for switcher input status: " + resp
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
	}

	// Remove any matrix formatting (this is also valid path for switchers)
	resp = strings.ReplaceAll(resp, `*`, ``)

	deviceModel := deviceModels[socketKey]

	inMap := make(map[string]int)

	switch {
	case strings.Contains(deviceModel, "DTPCP108"):
		inMap = crossPoint108Map.inputs
	case strings.Contains(deviceModel, "DTPCP86"):
		inMap = crossPoint86Map.inputs
	case strings.Contains(deviceModel, "DTPCP84"):
		inMap = crossPoint84Map.inputs
	case strings.Contains(deviceModel, "IN18"):
		inMap = in180xMap.inputs
	default:
		// If we got here, hopefully it's a device with a straight 1:1 mapping (ex: no '3A', just '3')
		framework.Log(function + " - no special I/O name handling applied for device: " + deviceModel + "at" + socketKey)
		inputNum, err := strconv.Atoi(input)
		if err != nil {
			errMsg := function + " - invalid input number: " + input
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
		// Check if index is in bounds
		if inputNum < 1 || inputNum > len(resp) {
			errMsg := function + " - input number out of range: " + input
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
		// Extract the single character
		singleCharResult := string(resp[inputNum-1])
		result, err := stringIntToStringBool(socketKey, singleCharResult)
		if err != nil {
			framework.AddToErrors(socketKey, err.Error())
			return "", err
		}
		return result, nil
	}

	// If we got here, we have a device that has a mapping for inputs (Ex: 3A is index 2, 3B is index 3)
	// Check if input is in the map
	var index int
	var ok bool
	if index, ok = inMap[input]; ok {
		result := string(resp[index])
		//framework.Log(fmt.Sprintf("%s - %s - input: %s, is at index: %d of %s", function, socketKey, input, index, resp))
		//framework.Log(fmt.Sprintf("%s - %s - result: %s", function, socketKey, result))
		return stringIntToStringBool(socketKey, result)
	} else {
		errMsg := function + " - invalid input name: " + input
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
}

func getVideoMuteDo(socketKey string, endpoint string, output string, _ string, _ string) (string, error) {
	function := "getVideoMuteDo"

	resp, err := deviceTypeDependantCommand(socketKey, "videomute", "GET", "", "", "")
	if err != nil {
		errMsg := function + "- error getting video mute status: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// all return strings contain "0" (not muted), "1" (muted with sync) or "2" (sync mute) for each output

	// DA returns string with single space between the characters, local HDMI loop through may be the first
	// Matrix with "A" or "B" ex CP84: "0 0 0 0 0 0" which is 1,2,3A,3B,4A,4B
	// Scaler with loopout (IN 1808) : "0 0 0", which is 1A,1B,LoopOut
	// Scaler with mirrored but individually-mute-controlled outs (IN 1804) : "0 0"
	// Scaler (IN 16xx) with standard mirrored outputs, just 0,1, or 2

	resp = strings.ReplaceAll(resp, " ", "")
	resp = strings.ReplaceAll(resp, `"`, ``)

	deviceType, err := findDeviceType(socketKey)
	if err != nil {
		errMsg := fmt.Sprintf(function+" - a9ebb - error finding device type: %s", err.Error())
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Simple device, only one character reply
	// Could be IN 16xx or a switcher
	if len(resp) == 1 {
		if resp == "0" {
			return "false", nil
		} else if resp == "1" {
			return "true", nil
		} else if resp == "2" { // sync mute
			return "true", nil
		} else {
			errMsg := function + " - invalid one character response for video mute: " + resp
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
	}

	// Query is for loop out (built for IN 1808)
	// Future maintainers: any new device with LoopOut needs to be handled here
	if output == "LoopOut" {
		framework.Log(fmt.Sprintf("%s - %s - LoopOut response: %s", function, socketKey, resp))
		if len(resp) == 3 {
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
		} else { // 1808 is only known LoopOut device, this was called on the wrong device, or we need update handling
			framework.AddToErrors(socketKey, function+" - LoopOut called, but device is not IN1808 "+resp)
		}
	}

	// Distribution Amplifier
	if deviceType == "Distribution Amplifier" {

		// assuming Extron will not make an odd number of outputs without a loop through
		hasLoopThrough := false
		if !isEven(len(resp)) {
			hasLoopThrough = true
		}

		if output == "LoopThrough" && hasLoopThrough {
			// LoopThrough is the first output in the string
			result := string(resp[0])
			switch result {
			case "0":
				return "false", nil
			case "1":
				return "true", nil
			case "2":
				return "true", nil
			default:
				errMsg := function + " - invalid loopthrough response: " + resp
				framework.AddToErrors(socketKey, errMsg)
				return errMsg, errors.New(errMsg)
			}
		} else if output == "LoopThrough" && !hasLoopThrough {
			// LoopThrough is not available on this device
			errMsg := function + " - LoopThrough not available on this device: " + resp
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}

		// Drop the first character (loop through)
		if len(resp) > 0 {
			resp = resp[1:]
		}
		outputInt, err := strconv.Atoi(output)
		if err != nil {
			errMsg := function + " - invalid output number: " + output
			framework.AddToErrors(socketKey, errMsg)
			return errMsg, errors.New(errMsg)
		}
		result := string(resp[outputInt-1]) //-1: convert for 0-based index
		switch result {
		case "0":
			return "false", nil
		case "1":
			return "true", nil
		case "2":
			return "true", nil
		}
	}

	// Matrix switchers and IN 180x
	// We need to map the output name to the index in the response string
	outMap := make(map[string]int)
	deviceModel := deviceModels[socketKey]

	switch {
	case strings.Contains(deviceModel, "DTPCP108"):
		outMap = crossPoint108Map.outputs
	case strings.Contains(deviceModel, "DTPCP86"):
		outMap = crossPoint86Map.outputs
	case strings.Contains(deviceModel, "DTPCP84"):
		outMap = crossPoint84Map.outputs
	case strings.Contains(deviceModel, "IN18"):
		outMap = in180xMap.outputs
	default:
		errMsg := function + " - unknown device model: " + deviceModels[socketKey]
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Check if output is in the map
	var index int
	var ok bool
	var result string
	if index, ok = outMap[output]; ok {
		result = string(resp[index])
	} else {
		errMsg := function + " - invalid output name: " + output
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// If we got here, we have a valid result

	//framework.Log(fmt.Sprintf("%s - %s - output: %s, is at index: %d of %s", function, socketKey, output, index, resp))
	//framework.Log(fmt.Sprintf("%s - %s - result: %s", function, socketKey, result))

	result, err = stringIntToStringBool(socketKey, result)
	if err != nil {
		errMsg := function + " - error converting video mute status to boolean: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	return result, nil
}

func getMatrixMuteDo(socketKey string, endpoint string, input string, output string, _ string) (string, error) {
	function := "getMatrixMuteDo"

	mixPointNumber, err := calculateDmpMixPointNumber(input, output)
	if err != nil {
		errMsg := function + " - error calculating mix point number: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	resp, err := deviceTypeDependantCommand(socketKey, "matrixmute", "GET", mixPointNumber, "", "")
	if err != nil {
		errMsg := function + "- error getting matrix mute status: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	resp = strings.ReplaceAll(resp, `"`, ``)
	if resp == "1" {
		return "true", nil
	} else if resp == "0" {
		return "false", nil
	} else {
		errMsg := function + " - invalid response for matrix mute: " + resp
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
}

func getMatrixVolumeDo(socketKey string, endpoint string, input string, output string, _ string) (string, error) {
	function := "getMatrixVolumeDo"

	mixPointNumber, err := calculateDmpMixPointNumber(input, output)
	if err != nil {
		errMsg := function + " - error calculating mix point number: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	resp, err := deviceTypeDependantCommand(socketKey, "matrixvolume", "GET", mixPointNumber, "", "")
	if err != nil {
		errMsg := function + "- error getting matrix volume status: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Valid returns are in 10th of DB's.  Ex: -3.5db is '-35'. Range: -100db to 12db.
	// Presume the rest of the system wants to get a 0-100 value where 0 is -100db and 100 is 12db
	// Convert device value (tenths dB) to a 0-100 percentage (logarithmic mapping)
	resp = strings.ReplaceAll(resp, `"`, ``)
	percent, convErr := newUnTransformVolume(resp)
	if convErr != nil {
		errMsg := function + " - error converting device volume to percent: " + convErr.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	return percent, nil
}

// Set functions //

// This endpoint has limited functionality within the Extron ecosystem.
// It is only used for group volume control of scalers.
// For dedicated DSP units and matrix switchers, you'll want to use "matrixvolume" instead.
// Note: IN1804 breaks all the patterns and is not supported yet.
func setVolumeDo(socketKey string, endpoint string, name string, level string, _ string) (string, error) {
	function := "setVolumeDo"

	model, err := findModelName(socketKey)
	if err != nil {
		modelErr := function + " - can not find model for: " + socketKey
		framework.AddToErrors(socketKey, modelErr)
		return modelErr, errors.New(modelErr)
	}

	var oid string // mix point number
	ok := false

	// Check if model is supported
	switch {
	case strings.Contains(model, "160") && strings.Contains(model, "IN"): // IN 160x series
		oid, ok = in160xGroupAudioVolumeMap[name] // Only group voloumes on 160x series (firmware bug)
	default:
		notImpMsg := function + "Model: " + model + " is not implemented or does not support 'volume'"
		framework.AddToErrors(socketKey, notImpMsg)
		return notImpMsg, errors.New(notImpMsg)
	}

	// Check if we have the channel name+oid mapping for the model
	if !ok {
		errMsg := function + "Can't find OID for: " + name + " on model: " + model
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Convert percent to device volume (tenths of DB)
	deviceVolume, err := newTransformVolume(level)
	if err != nil {
		errMsg := function + " - error converting percent to device volume: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Try sending the command
	resp, err := deviceTypeDependantCommand(socketKey, "volume", "SET", oid, deviceVolume, "")
	if err != nil {
		errMsg := function + " - error setting volume " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Good response is "GrpmD<mixPointNumber>*<volume>"
	resp = strings.ReplaceAll(resp, `"`, ``)
	expectedResp := "GrpmD" + oid + "*" + deviceVolume
	if resp != expectedResp {
		errMsg := function + " - invalid response for setting volume: " + resp + ", expected: " + expectedResp
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	return "ok", nil
}

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

func setVideoMuteDo(socketKey string, endpoint string, output string, state string, _ string) (string, error) {
	function := "setVideoMuteDo"

	var cmd string
	if state == "true" {
		cmd = "videomute"
	} else {
		cmd = "videounmute"
	}

	// DA with loop through, loop through is output "0"
	// Matrix switchers need to refer to output by name (ex: "3B")
	// IN 180x needs to refer to output by number (ex: 1B would be "2")
	// Non IN 180x Scalers or switchers can just call "1"
	resp, err := deviceTypeDependantCommand(socketKey, cmd, "SET", output, "", "")
	if err != nil {
		errMsg := function + "- error setting video mute: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Good response for 'all' devices: Vmt(output int)*(status int)
	if strings.Contains(resp, "Vmt") && strings.Contains(resp, output) {
		return "ok", nil
	} else {
		errMsg := function + " - invalid response for video mute: " + resp
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
}

func setVideoSyncMuteDo(socketKey string, endpoint string, output string, state string, _ string) (string, error) {
	function := "setVideoSyncMuteDo"

	var cmd string
	if state == "true" {
		cmd = "videosyncmute"
	} else {
		cmd = "videounmute"
	}

	// DA with loop through, loop through is output "0"
	// Matrix switchers need to refer to output by name (ex: "3B")
	// IN 180x needs to refer to output by number (ex: 1B would be "2")
	// Non IN 180x Scalers or switchers can just call "1"
	resp, err := deviceTypeDependantCommand(socketKey, cmd, "SET", output, "", "")
	if err != nil {
		errMsg := function + "- error setting video sync mute: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Good response for 'all' devices: Vmt(output int)*(status int)
	if strings.Contains(resp, "Vmt") && strings.Contains(resp, output) {
		return "ok", nil
	} else {
		errMsg := function + " - invalid response for video sync mute: " + resp
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
}

// Built for DMP DSP's
// Note, this is a 3 arg funciton.  The last argument (state) true or false is in the request body
// Ex: curl -X PUT "http://<containerIP>/telnet|admin:pw@<deviceAddr>/matrixmute/MicToOut3/4" -H "Content-Type: application/json" -d true
func setMatrixMuteDo(socketKey string, endpoint string, input string, output string, state string) (string, error) {
	function := "setMatrixMuteDo"

	if state == "" || state == "null" || state == "\"null\"" {
		emptyStateMsg := function + "- Arg3 is required but not found in the request body"
		framework.AddToErrors(socketKey, emptyStateMsg)
		return emptyStateMsg, errors.New(emptyStateMsg)
	}
	var cmdState string
	switch {
	case strings.Contains(state, "false"):
		cmdState = "0"
	case strings.Contains(state, "true"):
		cmdState = "1"
	default:
		stateErrMsg := function + " - Arg 3 must be 'true', or 'false'.  Got: " + state
		framework.AddToErrors(socketKey, stateErrMsg)
		return stateErrMsg, errors.New(stateErrMsg)
	}

	mixPointNumber, err := calculateDmpMixPointNumber(input, output)
	if err != nil {
		errMsg := function + " - error calculating mix point number: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	resp, err := deviceTypeDependantCommand(socketKey, "matrixmute", "SET", mixPointNumber, cmdState, "")
	if err != nil {
		errMsg := function + "- error getting matrix mute status: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Successful response is "DsM<mixPointNumber>*<1|0>"
	if strings.Contains(resp, "DsM") && strings.Contains(resp, cmdState) && strings.Contains(resp, mixPointNumber) {
		return "ok", nil
	} else {
		badRespMsg := function + " - unexpected device response: " + resp
		framework.AddToErrors(socketKey, badRespMsg)
		return badRespMsg, errors.New(badRespMsg)
	}
}

func setAudioMuteDo(socketKey string, endpoint string, output string, state string, _ string) (string, error) {
	function := "setAudioMuteDo"

	var cmd string
	if state == "true" {
		cmd = "audiomute"
	} else {
		cmd = "audiounmute"
	}

	resp, err := deviceTypeDependantCommand(socketKey, cmd, "SET", output, "", "")
	if err != nil {
		errMsg := function + "- error setting audio mute: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
	return resp, nil
}

// Note, this is a 3 arg funciton.  The last argument (level 0-100) is in the request body
// Ex: curl -X PUT "http://<containerIP>/telnet|admin:pw@<deviceAddr>/matrixvolume/MicToOut3/4" -H "Content-Type: application/json" -d 76
func setMatrixVolumeDo(socketKey string, endpoint string, input string, output string, level string) (string, error) {
	function := "setMatrixVolumeDo"

	mixPointNumber, err := calculateDmpMixPointNumber(input, output)
	if err != nil {
		errMsg := function + " - error calculating mix point number: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Framework enqueues arg3 wrapped in quotes (e.g., "50"). Sanitize before converting.
	levelSanitized := strings.TrimSpace(strings.Trim(level, `"`))
	if levelSanitized == "" {
		errMsg := function + " - level (0-100) required in request body"
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	levelVal, err := newTransformVolume(levelSanitized)
	if err != nil {
		errMsg := function + " - error converting percent volume to device volume: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	resp, err := deviceTypeDependantCommand(socketKey, "matrixvolume", "SET", mixPointNumber, levelVal, "")
	if err != nil {
		errMsg := function + "- error setting matrix volume: " + err.Error()
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

	// Valid response is "DsG<mixPointNumber>*<levelVal>"
	if strings.Contains(resp, "DsG") && strings.Contains(resp, mixPointNumber) && strings.Contains(resp, levelVal) {
		return "ok", nil
	} else {
		errMsg := function + " - invalid response for setting matrix volume: " + resp
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}
}

///////////////////////////////////////////////////////////////////////////////
// Helper functions //
///////////////////////////////////////////////////////////////////////////////

func isEven(n int) bool {
	return n%2 == 0
}

// The following two functions convert between:
// - Device volume in tenths-of-dB (range -1000 to +120 representing -100 dB to +12 dB)
// - Our API volume in 0-100 percent using a logarithmic curve (similar feel to Shure). Unity gain at 76
// Mapping is normalized so 0% -> -1000 and 100% -> +120, with a curve parameter k controlling shape.
// Note: 1804 does not follow the pattern and uses 1db steps on dbFS, -100 to +000. Ugh!

// Convert percent (0 to 100) into Extron-style tenth's of decibels
func newTransformVolume(percent string) (string, error) {
	function := "newTransformVolume"
	// percent is expected 0-100 (string)
	// sanitize input in case it arrives quoted
	percent = strings.TrimSpace(strings.Trim(percent, `"`))
	percentInt, err := strconv.Atoi(percent)
	if err != nil {
		return "", errors.New(fmt.Sprintf("%s - error converting percent to int: %v", function, err))
	}
	if percentInt < 0 {
		percentInt = 0
	}
	if percentInt > 100 {
		percentInt = 100
	}
	// Bounds in tenths dB
	minTenthsDb := -1000.0
	maxTenthsDb := 120.0
	tenthsDbRange := maxTenthsDb - minTenthsDb // 1120
	// Log curve similar to Shure, normalized to [0,1]
	k := 11.0
	denom := math.Log10(1.0 + 100.0/k)
	var normalized float64
	if denom == 0 {
		normalized = float64(percentInt) / 100.0
	} else {
		normalized = math.Log10(1.0+float64(percentInt)/k) / denom
	}
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	tenthsDb := minTenthsDb + normalized*tenthsDbRange
	tenthsDbInt := int(math.Round(tenthsDb))
	return strconv.Itoa(tenthsDbInt), nil
}

// Converts Extron-style tenths of decibels to percentage 0 to 100
func newUnTransformVolume(tenthsDb string) (string, error) {
	function := "newUnTransformVolume"
	// tenthsDb is a string like "-35" (=-3.5 dB) or "-1000" (=-100 dB)

	// sanitize input in case it arrives quoted from framework
	tenthsDb = strings.TrimSpace(strings.Trim(tenthsDb, `"`))
	tenthsDbInt, err := strconv.Atoi(tenthsDb)
	if err != nil {
		return "", errors.New(fmt.Sprintf("%s - error converting tenths dB to int: %v", function, err))
	}
	minTenthsDb := -1000.0
	maxTenthsDb := 120.0
	tenthsDbRange := maxTenthsDb - minTenthsDb // 1120
	// Clamp to expected device range
	clampedTenthsDb := float64(tenthsDbInt)
	if clampedTenthsDb < minTenthsDb {
		clampedTenthsDb = minTenthsDb
	}
	if clampedTenthsDb > maxTenthsDb {
		clampedTenthsDb = maxTenthsDb
	}
	// Normalize to [0,1]
	normalized := (clampedTenthsDb - minTenthsDb) / tenthsDbRange
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	// Invert the normalized log curve back to percent
	k := 11.0
	base := 1.0 + 100.0/k
	percent := k * (math.Pow(base, normalized) - 1.0)
	// Round to nearest integer and clamp 0..100
	percentInt := int(math.Round(percent))
	if percentInt < 0 {
		percentInt = 0
	}
	if percentInt > 100 {
		percentInt = 100
	}
	return strconv.Itoa(percentInt), nil
}

// Placeholder for not implemented functions
func notImplemented(socketKey string, endpoint string, _ string, _ string, _ string) (string, error) {
	function := "notImplemented"

	errMsg := fmt.Sprintf("%s - %s - endpoint '%s' is not implemented", function, socketKey, endpoint)
	framework.AddToErrors(socketKey, errMsg)
	return "", errors.New(errMsg)
}

func stringIntToStringBool(socketKey string, input string) (string, error) {
	// 'Casts' a string representation of an integer to a string representation of a boolean
	function := "stringIntToStringBool"

	result := ""
	errMsg := ""

	if input == "1" {
		result = "true"
	} else if input == "0" {
		result = "false"
	} else {
		errMsg = function + " - can't cast to 'True' or 'False': " + input
		return "", errors.New(errMsg)
	}

	return result, nil
}

///////////////////////////////////////////////////////////////////////////////
// Internal functions //
///////////////////////////////////////////////////////////////////////////////

// General internal functions //

// Internal
func telnetLoginNegotiation(socketKey string) (success bool) {
	function := "telnetLoginNegotiation"

	framework.Log(function + " - Starting telnet login for: " + socketKey)
	// Get password. Extron Telnet connection assumes 'admin' as username
	password := "" // device expects empty string if no password is set
	if strings.Count(socketKey, "@") == 1 {
		credentials := strings.Split(socketKey, "@")[0]
		if strings.Count(credentials, ":") == 1 {
			password = strings.Split(credentials, ":")[1]
		}
	}

	count := 0
	modelNameFound := false
	// Breaks if the negotiations go over 7 rounds to avoid an infinite loop.
	for count < 7 {
		count += 1
		negotiationResp := framework.ReadLineFromSocket(socketKey)

		// make use of the information presented at the login screen
		// Needed because not every device has a command to return model name, but they present it here
		if !modelNameFound {
			commas := strings.Count(negotiationResp, ",")
			if commas == 4 { // copywright, company, model name, firmware, part number
				modelName := strings.TrimSpace(strings.Split(negotiationResp, ",")[2])
				framework.Log(function + "- Model name: " + modelName)
				deviceModels[socketKey] = modelName
				modelNameFound = true
			} else if commas != 1 || commas != 0 { // unexpected response (date line contains 1 comma)
				framework.AddToErrors(socketKey, function+" - Help! does this line contain the model name? "+negotiationResp)
			}
		}

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
		protocol := framework.GetDeviceProtocol(socketKey)
		if protocol != "ssh" {
			negotiation := telnetLoginNegotiation(socketKey)
			if negotiation == false {
				errMsg := fmt.Sprintf(function + " - h3boid - error logging in")
				framework.AddToErrors(socketKey, errMsg)
				return errors.New(errMsg)
			}
		}
	}
	if framework.KeepAlivePolling {
		// startKeepAlivePoll will not add new goroutines if they already exist for the socketKey
		startKeepAlivePoll(socketKey, keepAlivePollingInterval, keepAliveCmd)
	}
	return nil
}

// Internal: Checks if the device returned an error code.  If it did, return a formatted error message.
// We don't raise Go errors here since we want the device response to be returned to the calling client
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

	deviceType := categorizeDeviceType(socketKey, resp)

	return deviceType, err
}

func categorizeDeviceType(socketKey string, modelDescriptionResp string) string {
	function := "categorizeDeviceType"

	deviceType := "unknown"

	resp := strings.ToLower(modelDescriptionResp)

	// TODO: commented out items and any additional device types
	switch {
	case strings.Contains(resp, "dmp") || strings.Contains(resp, "digital audio"):
		deviceType = "Audio Processor"

	case strings.Contains(resp, "presentation system"):
		deviceType = "Collaboration Systems" //ex: ShareLink. Note: SSH only

	case (strings.Contains(resp, "matrix") && !strings.Contains(resp, "audio")) || strings.Contains(resp, "xtp"):
		deviceType = "Matrix Switcher"

	case strings.Contains(resp, "scaling presentation switcher"):
		deviceType = "Scaler" // IN 16xx series

	case strings.Contains(resp, "seamless presentation switcher"):
		deviceType = "Scaler" // IN 18xx series (ex: 1804)

	case strings.Contains(resp, "seamless scaling switcher"):
		deviceType = "Scaler" // IN 18xx series (ex: 1808)

	case resp == "streaming media processor":
		deviceType = "Streaming Media" // ex: SMP3xx

	case resp == "collaboration switcher":
		deviceType = "Collaboration Switcher" //ex: UCS 303.

	case strings.Contains(resp, "switcher") && !strings.Contains(resp, "scaling") && !strings.Contains(resp, "matrix") && !strings.Contains(resp, "scaler"):
		deviceType = "Switcher" // non-scaling switchers, often older or low-end models

	case strings.Contains(resp, "distribution amplifier"):
		deviceType = "Distribution Amplifier"

	case strings.Contains(resp, "110v ac"):
		deviceType = "Power Controller"
	default:
		deviceType = "unknown"
	}

	deviceTypes[socketKey] = deviceType
	framework.Log(fmt.Sprintf("%s - %s - Device type determined: %s", function, socketKey, deviceType))

	return deviceType

}

// Internal: returns the model name from a package-level cache
// Model name is cached at initial connection as it's in the welcome banner
func findModelName(socketKey string) (string, error) {
	function := "findModelName"

	if modelName, exists := deviceModels[socketKey]; exists {
		framework.Log(fmt.Sprintf("%s - %s - Device model found in cache: %s", function, socketKey, modelName))
		return modelName, nil // cache hit
	}

	// It's possible we don't have a connection to the device yet.  Try connect then try again.
	err := ensureActiveConnection(socketKey)
	_ = err
	if modelName, exists := deviceModels[socketKey]; exists {
		framework.Log(fmt.Sprintf("%s - %s - Device model found in cache: %s", function, socketKey, modelName))
		return modelName, nil // cache hit
	}

	return "", errors.New("model name not found in cache")
}

// Internal: begins a periodic polling loop to keep the connection alive if one does not exist
func startKeepAlivePoll(socketKey string, interval time.Duration, keepAliveCmd string) error {
	function := "startKeepAlivePoll"

	keepAlivePollRoutinesMutex.Lock()
	defer keepAlivePollRoutinesMutex.Unlock()

	// Check if already running
	if _, exists := keepAlivePollRoutines[socketKey]; exists {
		return nil
	}

	if framework.SSHMode == "per-command session" && framework.GetDeviceProtocol(socketKey) == "ssh" {
		return nil
	}

	// Create stop channel
	stopCh := make(chan bool)
	keepAlivePollRoutines[socketKey] = stopCh

	// Start keepalive goroutine
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		framework.Log(fmt.Sprintf("%s - started for %s with interval %v", function, socketKey, interval))

		for {
			select {
			case <-ticker.C:
				resp, err := sendBasicCommand(socketKey, keepAliveCmd)
				if err != nil {
					framework.AddToErrors(socketKey, fmt.Sprintf("%s - failed: %v", function, err))
				}
				if resp == "" || strings.Contains(resp, "error") {
					framework.AddToErrors(socketKey, fmt.Sprintf("%s - unexpected keepalive response: %s", function, resp))
				}
			case <-stopCh:
				framework.Log(fmt.Sprintf("%s - stopped for %s", function, socketKey))
				return
			}
		}
	}()

	return nil
}

// stops all running keepalive routines
func stopAllKeepAlivePolling() (string, error) {
	function := "stopAllKeepAlivePolling"

	framework.KeepAlivePolling = false

	keepAlivePollRoutinesMutex.Lock()
	defer keepAlivePollRoutinesMutex.Unlock()

	for socketKey, stopCh := range keepAlivePollRoutines {
		close(stopCh)
		delete(keepAlivePollRoutines, socketKey)
		framework.Log(fmt.Sprintf("%s - stopped for %s", function, socketKey))
	}
	return "ok", nil
}

// re-enables the polling flag
// polling will resume on the next command per device
func restartKeepAlivePolling() (string, error) {
	function := "restartKeepAlivePolling"

	framework.KeepAlivePolling = true
	framework.Log(fmt.Sprintf("%s - polling will resume on next command per device", function))
	return "ok", nil

}

// Main function that handles device type dependent commands
func deviceTypeDependantCommand(socketKey string, endpoint string, method string, arg1 string, arg2 string, arg3 string) (string, error) {
	function := "deviceTypeDependantCommand"

	deviceType, err := findDeviceType(socketKey)
	if err != nil {
		errMsg := fmt.Sprintf(function+" - a9ebb - error finding device type: %s", err.Error())
		framework.AddToErrors(socketKey, errMsg)
		return errMsg, errors.New(errMsg)
	}

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

	cmdTemplate := cmdMap[endpoint][deviceType]
	cmdString := ""
	cmdString = formatCommand(cmdTemplate, arg1, arg2, arg3)

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

// Internal helper function to process SSH output
// Returns the last line to ignore welcome headers
func processSSHOutput(output string) string {
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	// Find the last non-empty line
	var lastLine string
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			lastLine = trimmed
			break
		}
	}
	return lastLine
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

// Returns the RxTx mutex per socket key. Needed to keep Rx/Tx atomic
func getSocketMutex(socketKey string) *sync.Mutex {
	if m, ok := txRxMutexes.Load(socketKey); ok {
		return m.(*sync.Mutex)
	}
	m := &sync.Mutex{}
	actual, _ := txRxMutexes.LoadOrStore(socketKey, m)
	return actual.(*sync.Mutex)
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

	mu := getSocketMutex(socketKey)
	mu.Lock()
	defer mu.Unlock()

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

	if framework.GetDeviceProtocol(socketKey) == "ssh" {
		resp = processSSHOutput(resp)
	}

	deviceErrMsg := formatDeviceErrMessage(socketKey, resp)
	if deviceErrMsg != "" {
		framework.AddToErrors(socketKey, deviceErrMsg)
		resp = deviceErrMsg // Return the error message as the response
	}

	resp = strings.TrimPrefix(resp, `"`)
	resp = strings.TrimSuffix(resp, `"`)

	return `"` + resp + `"`, nil
}
