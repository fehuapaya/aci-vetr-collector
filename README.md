<p align="center">
<img src="logo.png" width="417" height="84" border="0" alt="ACI vetR collector">
<br/>
ACI health check data collector
<p>
<hr/>

This tool collects data from the APIC to be used by Cisco Services in the ACI Health Check.

Binary releases are available [in the releases tab](https://github.com/brightpuddle/aci-vetr/releases). Please always use the latest release unless explicitly instructed to use an earlier release by Cisco Services.



## Purpose

This tool performs data collection for the ACI health check. This tool can be run by the Cisco Business Critical Services customer or coordinated with services to collect the data over WebEx.

Once the collection is complete, the tool will create a `aci-vetr-data.zip` file. This file should be provided to the Cisco Services ACI consulting engineer for further analysis.

The tool also creates an `aci-vetr-c.log` file that can be reviewed and/or provided to Cisco to troubleshoot any issues with the collection process. Note, that this file will only be available in a failure scenario; upon successful collection this file is bundled into the `aci-vetr-data.zip` file along with collection data.

## How it works

The tool collects data from a number of endpoints on the APIC for configuration, current faults, scale-related data, etc. The tool currently has no interaction with the switches--all data is collected from the APIC, via the API. All queries are for specific MO clases, which can be viewed within the source code.


## Usage

All command line paramters are optional; the tool will prompt for any missing information. This is a command line tool, but can be run directly from the Windows/Mac/Linux GUI if desired--the tool will pause once complete, before closing the terminal.

```
Usage: aci-vetr-c [--ip IP] [--username USERNAME] [--password PASSWORD]

Options:
  --ip IP, -i IP         APIC IP address
  --username USERNAME, -u USERNAME
  --password PASSWORD, -p PASSWORD
  --help, -h             display this help and exit
  --version              display version and exit
```
