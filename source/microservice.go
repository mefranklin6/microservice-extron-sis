package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Dartmouth-OpenAV/microservice-framework/framework"
)

func setFrameworkGlobals() {
	// globals that change modes in the microservice framework:
	framework.MicroserviceName = "OpenAV Extron SIS MicroService"
	framework.DefaultSocketPort = 23 // Default telnet port is 23
	framework.CheckFunctionAppendBehavior = "Remove older instance"
	framework.UseTelnet = true
	framework.KeepAlive = true
	framework.DeviceWillCloseConnection = true

	framework.RegisterMainGetFunc(doDeviceSpecificGet)
	framework.RegisterMainSetFunc(doDeviceSpecificSet)
}

var ErrorResponsesMap = map[string]string{
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

var getCommandsMap = map[string]string{
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

	"viewvideosignalpresence": "\x1B0LS\r", // non-matrix
	"viewallinputconnections": "0LS\r",     // matrix
	"viewvideoinput":          "&\r",       // non-matrix
	"viewaudioinput":          "$\r",       // non-matrix
	"viewcurrentinput":        "!\r",       // non-matrix
	"viewloopoutinput":        "\x1BLOUT\r",
	"viewallvideoties":        "\x1B0*1*1VC\r", // matrix
	"viewallaudioties":        "\x1B0*1*2VC\r", // matrix
	"viewmutestatus":          "%s*B\r",        // non-matrix
	"viewoutputvideomutes":    "\x1BVM\r",      // matrix

	"viewinputname":         "\x1B%sNI\r",    // arg1: input name
	"queryhdcpinputstatus":  "\x1BI%sHDCP\r", // arg1: input name
	"queryhdcpoutputstatus": "\x1BO%sHDCP\r", // arg1: output name
}

var setCommandsMap = map[string]string{
	"globalvideomute":        "1*B\r",
	"globalvideoandsyncmute": "2*B\r",
	"globalvideounmute":      "0*B\r",

	"lockallfrontpanelfunctions":      "1X\r",
	"lockadvancedfrontpanelfunctions": "2X\r",
	"unlockallfrontpanelfunctions":    "0X\r",

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

// Every microservice using this golang microservice framework needs to provide this function to invoke functions to do sets.
// socketKey is the network connection for the framework to use to communicate with the device.
// setting is the first parameter in the URI.
// arg1 are the second and third parameters in the URI.
//
//	  Example PUT URIs that will result in this function being invoked:
//		 ":address/:setting/"
//	  ":address/:setting/:arg1"
//	  ":address/:setting/:arg1/:arg2"
func doDeviceSpecificSet(socketKey string, setting string, arg1 string, arg2 string, arg3 string) (string, error) {
	function := "doDeviceSpecificSet"

	if command, exists := setCommandsMap[setting]; exists {
		command = formatCommand(command, arg1, arg2, arg3)
		return sendBasicCommand(socketKey, command)
	}

	// Add a case statement for commands that require special processing.
	// These calls can use 0, 1, or 2 arguments.

	//switch setting {
	//case "special1":
	//	return setSpecial1(socketKey, arg1, arg2)
	//case "special2":
	//	return setSpecial2(socketKey, arg1, arg2)
	//}

	// If we get here, we didn't recognize the setting.  Send an error back to the config writer who had a bad URL.
	errMsg := function + " - unrecognized setting in URI: " + setting
	framework.AddToErrors(socketKey, errMsg)
	err := errors.New(errMsg)
	return setting, err
}

// Every microservice using this golang microservice framework needs to provide this function to invoke functions to do gets.
// socketKey is the network connection for the framework to use to communicate with the device.
// setting is the first parameter in the URI.
// arg1 are the second and third parameters in the URI.
//
//	  Example GET URIs that will result in this function being invoked:
//		 ":address/:setting/"
//	  ":address/:setting/:arg1"
//	  ":address/:setting/:arg1/:arg2"
func doDeviceSpecificGet(socketKey string, setting string, arg1 string, arg2 string) (string, error) {
	function := "doDeviceSpecificGet"

	if command, exists := getCommandsMap[setting]; exists {
		command = formatCommand(command, arg1, arg2, "")
		return sendBasicCommand(socketKey, command)
	}

	// Add a case statement for commands that require special processing.
	// These calls can use 0, 1, or 2 arguments.

	//switch setting {
	//case "special1":
	//	return getSpecial1(socketKey, arg1, arg2)
	//case "special2":
	//	return getSpecial2(socketKey, arg1, arg2)
	//}

	// If we get here, we didn't recognize the setting.  Send an error back to the config writer who had a bad URL.
	errMsg := function + " - unrecognized setting in URI: " + setting
	framework.AddToErrors(socketKey, errMsg)
	err := errors.New(errMsg)
	return setting, err
}

func main() {
	setFrameworkGlobals()
	framework.Startup()
}
