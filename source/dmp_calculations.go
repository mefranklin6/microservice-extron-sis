package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DMP's are static-architecture DSP's that rely on a matrix of mix points
// This file handles the maths to determine mix point numbers
// These were built using a DMP 128 Plus C V AT

// Base addresses for DMP matrix tables. See the DMP user manual.
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
