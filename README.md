# ACI Health Check Collector

Collect data from the APIC to be used by Cisco Services in the ACI Health Check.

Releases are available here:

https://github.com/brightpuddle/aci-health-check-collector/releases

## Usage

All command line paramters are optional; the tool will prompt for any missing information.

```
Usage: aci-collector [--ip IP] [--username USERNAME] [--password PASSWORD]

Options:
  --ip IP, -i IP         APIC IP address
  --username USERNAME, -u USERNAME
  --password PASSWORD, -p PASSWORD
  --help, -h             display this help and exit
  --version              display version and exit
```

Once the collection is complete, the tool will create a `health-check-data.zip` file. Please provide this tool to your Cisco Services ACI consulting engineer for further analysis.

The tool also creates an `aci-collector.log` file that can be provided to Cisco to troubleshoot any issues with the collection process. Note, that this file will only be found in a failure scenario; upon successful collection this file is bundled into the `health-check-data.zip` file along with collection data.
