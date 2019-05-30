# ACI Vet collector

This tool collects data from the APIC to be used by Cisco Services in the ACI Health Check.

Releases are available here. Please always use the latest release unless explicitly instructed to use an earlier release by Cisco Services.

https://github.com/brightpuddle/aci-vet-collector/releases

## Purpose

This tool performs data collection for the *ACI vet* tool. This tool can be run by the Cisco customer or coordinated with services to collect the data over WebEx.

Once the collection is complete, the tool will create a `aci-vet-data.zip` file. This file should be provided to the Cisco Services ACI consulting engineer for further analysis.

The tool also creates an `aci-vet.log` file that can be provided to Cisco to troubleshoot any issues with the collection process. Note, that this file will only be found in a failure scenario; upon successful collection this file is bundled into the `aci-vet-data.zip` file along with collection data.

## How it works

The tool collects data from a number of endpoints on the APIC for configuration, current faults, scale-related data, etc. There's currently no interaction with the switches--all data is collected from the APIC, via the API.


## Usage

All command line paramters are optional; the tool will prompt for any missing information.

```
Usage: aci-vet-collector [--ip IP] [--username USERNAME] [--password PASSWORD]

Options:
  --ip IP, -i IP         APIC IP address
  --username USERNAME, -u USERNAME
  --password PASSWORD, -p PASSWORD
  --help, -h             display this help and exit
  --version              display version and exit
```
