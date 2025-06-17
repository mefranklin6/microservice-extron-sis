package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Dartmouth-OpenAV/microservice-framework/framework"
)

// Package-level variables
var deviceTypes = make(map[string]string)              // socketKey -> deviceType
var deviceModels = make(map[string]string)             // socketKey -> modeldescription
var keepAlivePollRoutines = make(map[string]chan bool) // socketKey -> stop channel
var keepAlivePollRoutinesMutex sync.Mutex

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

///////////////////////////////////////////////////////////////////////////////
// Helper functions //
///////////////////////////////////////////////////////////////////////////////

func isEven(n int) bool {
	return n%2 == 0
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

//////////////////////////////////////////

// Internal: acts as a router to the sub-helper functions for calculating DMP mix points
func calculateDmpMixPointNumber(input, output string) (string, error) {
	function := "calculateDmpMixPointNumber"

	var prefix string
	var mixPointNum string
	var err error

	switch {
	case strings.HasPrefix(input, "MicToOut"):
		prefix = "MicToOut"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = micInputToOutputNumber(input, output)
		if err != nil {
			return "", err
		}
	case strings.HasPrefix(input, "VRetToOut"):
		prefix = "VRetToOut"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = virtualReturnToOutput(input, output)
		if err != nil {
			return "", err
		}
	case strings.HasPrefix(input, "EXPInToOut"):
		prefix = "EXPInToOut"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = eXPInputToOutput(input, output)
		if err != nil {
			return "", err
		}
	case strings.HasPrefix(input, "MicToSend"):
		prefix = "MicToSend"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = micInputToVirtualSend(input, output)
		if err != nil {
			return "", err
		}
	case strings.HasPrefix(input, "VRetToSend"):
		prefix = "VRetToSend"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = virtualReturnToVirtualSend(input, output)
		if err != nil {
			return "", err
		}
	case strings.HasPrefix(input, "EXPInToSend"):
		prefix = "EXPInToSend"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = eXPInputToVirtualSend(input, output)
		if err != nil {
			return "", err
		}
	case strings.HasPrefix(input, "MicToEXPOut"):
		prefix = "MicToEXPOut"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = micInputToEXPOutput(input, output)
		if err != nil {
			return "", err
		}
	case strings.HasPrefix(input, "VRetToEXPOut"):
		prefix = "VRetToEXPOut"
		input = strings.TrimPrefix(input, prefix)
		mixPointNum, err = virtualReturnToEXPOutput(input, output)
		if err != nil {
			return "", err
		}
	default:
		errMsg := fmt.Sprintf(function+" - unknown input type: %s", input)
		return "", errors.New(errMsg)
	}
	return mixPointNum, nil
}

// Internal: calculates the mix-point number for the DMP
func dmpCalc(table TableKey, row int, col int) (string, error) {
	// get the base address for the table
	base, ok := dmpBaseAddr[table]
	if !ok {
		return "", fmt.Errorf("unknown table key %s", table)
	}

	// apply the DMP mix-point formula
	intResult := base + 100*row + col

	// convert back to string
	strResult := strconv.Itoa(intResult)
	if strResult == "" {
		return "", fmt.Errorf("input %s, output %s not supported", row, col)
	}
	return strResult, nil
}

// MicInputToOutput returns Table 3 mix-point: Mic/Line input → Line out
func micInputToOutputNumber(input, output string) (string, error) {
	inputInt, err := strconv.Atoi(input)
	if err != nil {
		return "", fmt.Errorf("input %s is not a number", input)
	}
	outputInt, err := strconv.Atoi(output)
	if err != nil {
		return "", fmt.Errorf("output %s is not a number", output)
	}

	res, err := dmpCalc(MicToOut, inputInt-1, outputInt-1)
	if err != nil {
		return "", fmt.Errorf("error calculating mix-point: %s", err)
	}

	return res, nil
}

// virtualReturnToOutput returns Table 4 mix-point: Virtual Return A-H → Line out
func virtualReturnToOutput(vret, output string) (string, error) {
	// Convert vret to a rune and validate it
	if len(vret) != 1 {
		return "", fmt.Errorf("return A-H expected, got %s", vret)
	}
	vretRune := rune(vret[0])
	if vretRune < 'A' || vretRune > 'H' {
		return "", fmt.Errorf("return A-H expected, got %c", vretRune)
	}

	// Convert output to an integer and validate it
	outputInt, err := strconv.Atoi(output)
	if err != nil {
		return "", fmt.Errorf("output %s is not a valid number", output)
	}
	if outputInt < 1 {
		return "", fmt.Errorf("output must be at least 1, got %d", outputInt)
	}

	// Perform the mix-point calculation
	return dmpCalc(VRetToOut, int(vretRune-'A'), outputInt-1)
}

// eXPInputToOutput returns Table 5 mix-point: EXP input → Line out
func eXPInputToOutput(expIn, output string) (string, error) {
	// Convert expIn and output to integers
	expInInt, err := strconv.Atoi(expIn)
	if err != nil {
		return "", fmt.Errorf("input %s is not a valid number", expIn)
	}
	outputInt, err := strconv.Atoi(output)
	if err != nil {
		return "", fmt.Errorf("output %s is not a valid number", output)
	}

	// Perform the mix-point calculation
	return dmpCalc(EXPInToOut, expInInt-1, outputInt-1)
}

// micInputToVirtualSend returns Table 6 mix-point: Mic/Line input → Send A-H
func micInputToVirtualSend(input string, send string) (string, error) {
	// Convert input to an integer
	inputInt, err := strconv.Atoi(input)
	if err != nil {
		return "", fmt.Errorf("input %s is not a valid number", input)
	}

	// Convert send to a rune and validate it
	if len(send) != 1 {
		return "", fmt.Errorf("send A-H expected, got %s", send)
	}
	sendRune := rune(send[0])
	if sendRune < 'A' || sendRune > 'H' {
		return "", fmt.Errorf("send A-H expected, got %c", sendRune)
	}

	// Perform the mix-point calculation
	return dmpCalc(MicToSend, inputInt-1, int(sendRune-'A'))
}

// virtualReturnToVirtualSend returns Table 7 mix-point: Virtual Return A-H → Send A-H
func virtualReturnToVirtualSend(vret, send string) (string, error) {
	// Convert vret to a rune and validate it
	if len(vret) != 1 {
		return "", fmt.Errorf("return A-H expected, got %s", vret)
	}
	vretRune := rune(vret[0])
	if vretRune < 'A' || vretRune > 'H' {
		return "", fmt.Errorf("return A-H expected, got %c", vretRune)
	}

	// Convert send to a rune and validate it
	if len(send) != 1 {
		return "", fmt.Errorf("send A-H expected, got %s", send)
	}
	sendRune := rune(send[0])
	if sendRune < 'A' || sendRune > 'H' {
		return "", fmt.Errorf("send A-H expected, got %c", sendRune)
	}

	// Perform the mix-point calculation
	return dmpCalc(VRetToSend, int(vretRune-'A'), int(sendRune-'A'))
}

// eXPInputToVirtualSend returns Table 8 mix-point: EXP input → Send A-H
func eXPInputToVirtualSend(expIn string, send string) (string, error) {
	// Convert expIn to an integer
	expInInt, err := strconv.Atoi(expIn)
	if err != nil {
		return "", fmt.Errorf("input %s is not a valid number", expIn)
	}

	// Convert send to a rune and validate it
	if len(send) != 1 {
		return "", fmt.Errorf("send A-H expected, got %s", send)
	}
	sendRune := rune(send[0])
	if sendRune < 'A' || sendRune > 'H' {
		return "", fmt.Errorf("send A-H expected, got %c", sendRune)
	}

	// Perform the mix-point calculation
	return dmpCalc(EXPInToSend, expInInt-1, int(sendRune-'A'))
}

// micInputToEXPOutput returns Table 9 mix-point: Mic/Line input → EXP out
func micInputToEXPOutput(input, expOut string) (string, error) {
	// Convert input and expOut to integers
	inputInt, err := strconv.Atoi(input)
	if err != nil {
		return "", fmt.Errorf("input %s is not a valid number", input)
	}
	expOutInt, err := strconv.Atoi(expOut)
	if err != nil {
		return "", fmt.Errorf("output %s is not a valid number", expOut)
	}

	// Perform the mix-point calculation
	return dmpCalc(MicToEXPOut, inputInt-1, expOutInt-1)
}

// virtualReturnToEXPOutput returns Table 10 mix-point: Virtual Return A-H → EXP out
func virtualReturnToEXPOutput(vret, expOut string) (string, error) {
	// Convert vret to a rune and validate it
	if len(vret) != 1 {
		return "", fmt.Errorf("return A-H expected, got %s", vret)
	}
	vretRune := rune(vret[0])
	if vretRune < 'A' || vretRune > 'H' {
		return "", fmt.Errorf("return A-H expected, got %c", vretRune)
	}

	// Convert expOut to an integer
	expOutInt, err := strconv.Atoi(expOut)
	if err != nil {
		return "", fmt.Errorf("output %s is not a valid number", expOut)
	}

	// Perform the mix-point calculation
	return dmpCalc(VRetToEXPOut, int(vretRune-'A'), expOutInt-1)
}

////////////////////////////////////

// Internal
func telnetLoginNegotiation(socketKey string) (success bool) {
	function := "telnetLoginNegotiation"

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
		if framework.UseTelnet {
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
		deviceType = "Scaler" // IN 18xx series

	case strings.Contains(resp, "seamless scaling switcher"):
		deviceType = "Scaler" // IN 18xx series

	case resp == "streaming media processor":
		deviceType = "Streaming Media" // ex: SMP3xx

	case strings.Contains(resp, "switcher") && !strings.Contains(resp, "scaling") && !strings.Contains(resp, "matrix") && !strings.Contains(resp, "scaler"):
		deviceType = "Switcher" // non-scaling switchers, often older or low-end models

	case strings.Contains(resp, "distribution amplifier"):
		deviceType = "Distribution Amplifier"
	default:
		deviceType = "unknown"
	}

	deviceTypes[socketKey] = deviceType
	framework.Log(fmt.Sprintf("%s - %s - Device type determined: %s", function, socketKey, deviceType))

	return deviceType

}

// Internal: returns the model name from a package-level cache
// We presume devices announce their model name in the negotiation phase
func findModelName(socketKey string) (string, error) {
	function := "findModelName"

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

	framework.Log(fmt.Sprintf("%s - Command template: %s", function, cmdTemplate))
	framework.Log(fmt.Sprintf("%s - Args before formatting: arg1=%s, arg2=%s, arg3=%s", function, arg1, arg2, arg3))

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
