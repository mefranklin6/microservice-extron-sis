package main

import (
	"errors"
	"time"

	"github.com/mefranklin6/microservice-framework/framework" // Change after PR#4 for Dartmouth
)

func setFrameworkGlobals() {
	// globals that change modes in the microservice framework:
	framework.MicroserviceName = "OpenAV Extron SIS MicroService"
	framework.DefaultSocketPort = 23 // Default telnet port is 23
	framework.CheckFunctionAppendBehavior = "Remove older instance"
	framework.UseTelnet = true
	framework.KeepAlive = true
	framework.KeepAlivePolling = true              // make framework aware we're implementing polling here
	framework.DisconnectAfterDoneRefreshing = true // or else devices will close

	framework.RegisterMainGetFunc(doDeviceSpecificGet)
	framework.RegisterMainSetFunc(doDeviceSpecificSet)
}

// Package-level tunables
var keepAlivePollingInterval = 60 * time.Second // default : 60 seconds
var keepAliveCmd = "Q\r"                        // default : "Q\r" (firmware version)
var maintenancePeriod = struct {                // lets the connection drop during this period, daily
	Start time.Time
	End   time.Time
}{
	Start: time.Date(0, 1, 1, 2, 0, 0, 0, time.UTC), // 2:00 AM
	End:   time.Date(0, 1, 1, 3, 0, 0, 0, time.UTC), // 3:00 AM
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

	if command, exists := publicSetCmdEndpoints[setting]; exists {
		command = formatCommand(command, arg1, arg2, arg3)
		return sendBasicCommand(socketKey, command)
	}

	// Add a case statement for commands that require special processing.
	// These calls can use 0, 1, or 2 arguments.

	switch setting {
	case "videoroute":
		return specialEndpointSet(socketKey, "videoroute", arg1, arg2, "") // arg1: output, arg2: input
	case "audioandvideoroute":
		return specialEndpointSet(socketKey, "audioandvideoroute", arg1, arg2, "") // arg1: output, arg2: input
	case "videomute":
		return specialEndpointSet(socketKey, "videomute", arg1, arg2, "") // arg1: output, arg2: bool
	case "videosyncmute":
		return specialEndpointSet(socketKey, "videosyncmute", arg1, arg2, "") // arg1: output, arg2: bool
	case "stopallkeepalivepolling":
		return stopAllKeepAlivePolling()
	case "restartkeepalivepolling":
		return restartKeepAlivePolling()
		//case "special1":
		//	return setSpecial1(socketKey, arg1, arg2)
		//case "special2":
		//	return setSpecial2(socketKey, arg1, arg2)
	}

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

	if command, exists := publicGetCmdEndpoints[setting]; exists {
		command = formatCommand(command, arg1, arg2, "")
		return sendBasicCommand(socketKey, command)
	}

	// Add a case statement for commands that require special processing.
	// These calls can use 0, 1, or 2 arguments.

	switch setting {
	case "videoroute":
		return specialEndpointGet(socketKey, "videoroute", arg1, "", "") // arg1: output (if not matrix, use '1' for arg1)
	case "inputstatus":
		return specialEndpointGet(socketKey, "inputstatus", arg1, "", "") // arg1: input
	case "videomute":
		return specialEndpointGet(socketKey, "videomute", arg1, "", "") // arg1: output (if not matrix, use '1' for arg1)
	case "matrixmute":
		return specialEndpointGet(socketKey, "matrixmute", arg1, arg2, "") // arg1: input, arg2: output)
	}

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
