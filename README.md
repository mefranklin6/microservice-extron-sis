# microservice-extron-sis

Universal OpenAV microservice for Extron devices that support Simple Instruction Set (SIS)

Supports telnet and serial connections

You may find the manual for your device helpful for finding endpoint names, as many commands retain the same name

## Notes

- Even though all modern Extron devices share the same instruction set, not all commands are valid for all devices.  You will receive an appropriate error if a device does not support a command.

- There is no available 'master sheet' containing all of the SIS commands for all devices.  The commands in this driver at release are an aggregate of the most userful commands from multiple device manuals, and they may not be complete
