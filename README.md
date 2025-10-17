# microservice-extron-sis

Universal [OpenAV](https://github.com/Dartmouth-OpenAV) compatible microservice for Extron devices that support Simple Instruction Set (SIS)

Not affiliated with Extron.  SIS and Simple Instruction Set are copyrights of Extron.

Work-in-progress

## Features

- Supports SSH, Telnet and serial connections.

- Supports the official OpenAV endpoints (work in progress)

- Supports extra endpoints such as `temperature`.  See `publicGetCmdEndpoints` and `publicSetCmdEndpoints` for supported commands.

## Contributing

### Creating a developer environment (Windows)

1. [Download Docker Desktop for Windows](https://www.docker.com/products/docker-desktop/).  Make a Docker personal account if you don't already have one.  Since this is an open-source project, Docker Desktop is available at no cost.  Please note that other uses of Docker Desktop may violate the free license.  You might be asked to install and configure WSL during the process.

2. Clone this repo into your IDE.  If using VSCode, allow it to download all the helpful Go features like the linter and allow it to run gofmt on save (default).

3. In a terminal, from the filepath of the cloned repo, initialize the shared microservice-framework with

    ```pwsh
    git submodule sync --recursive
    git submodule update --init --recursive --depth 1
    ```

4. When you're ready to run the container, execute the included helper script: `recompile.ps1`.  If you make any modifications to the code, just run the script again to make a new container.

### Adding endpoints

Endpoints follow the OpenAV Unified Endpoint Definitions file.

#### 1. In `src/mappings.go` Add device specific commands to `internalGetCmdMap` or `internalSetCmdMap` following the nested structure of

```json
[
    "endpointname": {
        "deviceType1": "dev1command",
        "deviceType2": "dev2command",
    }
]
```

> Be aware of Go's escape strings.  For example if a command includes a `%`, then it needs to be escaped by `%%`
>
> Hint: Many SIS commands use the "ESC" button, which is coded as `\x1B`.  Return is `\r`

Device Types are mostly derived from their GVE types, but we need to make exceptions if there's any differing commands.  If you need to add a device type, make sure to edit `findDeviceType` function accordingly.

Currently the valid device types are:

- `Audio Processor` (ex: DMP)
- `Collaboration Systems` (ex: Sharelink)
- `Controller` (power and relay control, not IPCP's)
- `Distribution Amplifier`
- `Matrix Switcher` (ex: CrossPoint)
- `Scaler` (ex: IN xx0x)
- `Streaming Media` (ex: SMP 3xx)
- `USB Collaboration Switcher` (ex: UCS 303)

#### 2. Make your function is under `// Get Functions //` or `// Set Functions //` in `driver.go`

Function name conventions: "get or set" + "endpoint name" + "Do"

```go
func getSomethingDo(socketKey string, endpoint string, arg1 string, arg2 string, arg3 string) (string, error) {
    // name of the function, used for logging
    function := "getSomethingDo"

    // send the command
    // the first parameter is always socketKey
    // the second parameter is the name of your endpoint
    // the third parameter is either "GET" or "SET"
    // the last three are endpoint arguments
    // for any args you don't need, just send "" (Go does not support optional params)
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

Build and run the docker image to run commands against it
Writing test scripts is encouraged.

### Testing

With the docker container running, the default settings expose port 80 on all network interfaces.
You can either use the localhost address `127.0.0.1` or you can use the IP of your network interface card from an external computer.

- Curl example to get the temperature of a device at 192.168.50.82:

    ```pwsh
    curl http://127.0.0.1/telnet|admin:password@192.168.50.82:23/temperature
    ```

Broken down:

- `curl` with no augments default to GET.  If you want to instead change a setting, you would use `curl -X PUT`.

- `http://127.0.0.1/`: The docker host address.  In this case it's localhost as we're sending curl commands from the machine running the docker container

- `telnet|username:password@192.168.50.82:23`:
  - Protocol followed by pipe `|`.  Some Extron devices are Telnet only, some are SSH only, some support both.  It is recommended to use Telnet if the device supports it.  If you don't specify a protocol, the framework will assume TCP, which will not work for Extron devices.
  
  - `username:password`: Generally you'll use the 'admin' account, although Extron also supports 'user' with less privileges.
  
  - `<ipaddress/DNS name>:<port>`.  If you don't specify the port, it will default to 23 if protocol is Telnet, or 22023 if SSH.
  
  - `/temperature` the endpoint to GET temperature.  If this call had any arguments to pass, you would specify up to three arguments afterwards with forward slashes `/`.

Returns the temperature string the device gave 'ex "35C", or an error string

- Curl example to set video mute ON for output 1 of a device at 192.168.50.82.  Note that we can omit the SSH port as the framework will fill in DefaultSSHPort set to the proper 22023:

    ```pwsh
    curl -X PUT http://127.0.0.1/ssh|admin:DEVICEPASSWORD@192.168.50.82/videomute/1/true
    ```

- ```pwsh/videomute/1/true```:
  - Endpoint: `videomute`
  - Arg1: output identifier (e.g., `1`, `1A`, `3B`, or `LoopThrough` depending on device type)
  - Arg2: desired state (`true` to mute, `false` to unmute)

Returns: "ok" or error string
