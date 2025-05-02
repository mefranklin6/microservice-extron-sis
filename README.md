# microservice-extron-sis

Universal OpenAV microservice for Extron devices that support Simple Instruction Set (SIS)

Not affliated with Extron.  SIS and Simple Instruction Set are copyrights of Extron.

## Features

- Supports telnet and serial connections.

- Supports the offical OpenAV endpoints (work in progress)

- Supports extra endpoints such as `temperature`.  See `publicGetCmdEndpoints` and `publicSetCmdEndpoints` for supported commands.

## Contributing

### Adding endpoints

Endpoints follow the OpenAV Unified Endpoint Definitions file.

#### 1. Add device specific commands to `internalGetCmdMap` or `internalSetCmdMap` following the nested structure of

```json
[
    "endpointname": {
        "deviceType1": "dev1command",
        "deviceType2": "dev2command",
    }
]
```

*Note: be aware of Go's escape strings.  For example if a command includes a `%`, then it needs to be escaped by `%%`*

Device Types are derived from their GVE types, but we need to make exceptions if there's any differing commands.  If you need to add a device type, make sure to edit `findDeviceType` function accordingly.

Currently the valid device types are:

- `Audio Processor` (ex: DMP)
- `Collabration Systems` (ex: Sharelink)
- `Controller` (power and relay control, not IPCP's)
- `Distribution Amplifier`
- `Matrix Switcher` (ex: CrossPoint)
- `Scaler` (ex: IN xx0x)
- `Streaming Media` (ex: SMP 3xx)

#### 2. Make your function under `// Get Functions //` or `// Set Functions //` in `driver.go`

Function name : get or set + endpoint name + Do

```go
func getSomethingDo(socketKey string, endpoint string, arg1 string, arg2 string, arg3 string) (string, error) {
    // name of the function, used for logging
    function := "getSomethingDo"

    // send the command
    // the first paramater is always sockeKey
    // the second paramater is the name of your endpoint
    // the third paramater is either "GET" or "SET"
    // the last three are endpoint arguments
    // for any args you don't need, just send ""
    resp, err := deviceTypeDependantCommand(socketKey, "endpointname", "GET", arg1, "", "")

    // check if there was a problem and log it
    if err != nil {
        errMsg := function + "- error getting something: " + err.Error()
        framework.AddToErrors(socketKey, errMsg)
        return errMsg, errors.New(errMsg)
    }

    // add any validation, filtering, or conversion here
    
    // Validation: make sure we received an expected response and check if the device returned an error message
    
    // Filtering: for example, if you can only query all channel names but you only want to return the n'th channel name, do that here.
    
    // Conversion: some Set commands should only return "true" or "false" or the err message, but the device may return an 1 or 0.  Convert that here.

    return resp, nil
}

```

#### 3. Add the endpoint to the switch in either `doDeviceSpecificSet` or `doDeviceSpecificGet`

return either `specialEndpointGet` or `specialEndpointSet` with

- socketKey
- name of the endpoint
- all three endpoint args (if needed, else you can `""`)

```go
    case "endpointname":
        return specialEndpointGet(socketKey, "endpointname", arg1, arg2, arg3)
```

#### 4. Test on real devices, as much as practical

Writing test scripts is encouraged.
