<p align="center">
<img src="logo.png" width="418" height="84" border="0" alt="ACI vetR collector">
<br/>
ACI health check data collector
<p>
<hr/>

This tool collects data from the APIC to be used by Cisco Services in the ACI Health Check.

Binary releases are available [in the releases tab](https://github.com/brightpuddle/aci-vetr/releases). Please always use the latest release unless explicitly instructed to use an earlier release by Cisco Services.



# Purpose

This tool performs data collection for the ACI health check. This tool can be run by the Cisco Business Critical Services customer or coordinated with services to collect the data over WebEx.

Once the collection is complete, the tool will create a `aci-vetr-data.zip` file. This file should be provided to the Cisco Services ACI consulting engineer for further analysis.

The tool also creates an `aci-vetr-c.log` file that can be reviewed and/or provided to Cisco to troubleshoot any issues with the collection process. Note, that this file will only be available in a failure scenario; upon successful collection this file is bundled into the `aci-vetr-data.zip` file along with collection data.

# How it works

The tool collects data from a number of endpoints on the APIC for configuration, current faults, scale-related data, etc. The results of these queries are archived in a zip file to be shared with Cisco. The tool currently has no interaction with the switches--all data is collected from the APIC, via the API.

The following API managed objects are queried by this tool:

```
/api/class/eqptcapacityL3RemoteUsageCap5min.json
/api/class/eqptcapacityL3UsageCap5min.json
/api/class/infraRsVlanNs.json
/api/class/topSystem.json
/api/class/infraSetPol.json
/api/class/fabricSetupP.json
/api/class/fvAEPg.json
/api/class/l3extLIfP.json
/api/class/bgpRRNodePEp.json
/api/class/epLoopProtectP.json
/api/class/firmwareCtrlrRunning.json
/api/class/eqptcapacityL2RemoteUsage5min.json
/api/class/eqptcapacityL2TotalUsage5min.json
/api/class/fvSubnet.json
/api/class/fvTenant.json
/api/class/eqptcapacityVlanUsage5min.json
/api/class/eqptcapacityPolUsage5min.json
/api/class/fabricNodeControl.json
/api/class/l3extOut.json
/api/class/eqptcapacityL2Usage5min.json
/api/class/fvCEp.json?rsp-subtree-include=count
/api/class/eqptcapacityL3TotalUsage5min.json
/api/class/fvRsCons.json
/api/class/fvIp.json?rsp-subtree-include=count
/api/class/vzRsSubjFiltAtt.json
/api/class/fvnsEncapBlk.json
/api/class/fabricRsLeNodePGrp.json
/api/class/vzBrCP.json
/api/class/eqptBoard.json
/api/class/firmwareRunning.json
/api/class/eqptcapacityMcastUsage5min.json
/api/class/fabricNode.json
/api/class/fvCtx.json
/api/class/epControlP.json
/api/class/l3extLNodeP.json
/api/class/fabricRsNodeCtrl.json
/api/class/vnsCDev.json?rsp-subtree-include=count
/api/class/isisDomPol.json
/api/class/vnsGraphInst.json?rsp-subtree-include=count
/api/class/eqptcapacityL3Usage5min.json
/api/class/fvRsProv.json
/api/class/l3extInstP.json
/api/class/eqptcapacityL3RemoteUsage5min.json
/api/class/infraAttEntityP.json
/api/class/fvBD.json
/api/class/fvRsBd.json
/api/class/epIpAgingP.json
/api/class/infraPortTrackPol.json
/api/class/l3extRsNodeL3OutAtt.json
/api/class/vzSubj.json
/api/class/ctxClassCnt.json?rsp-subtree-class=l2BD,fvEpP,l3Dom
/api/class/infraRsDomP.json
/api/class/mcpIfPol.json
/api/class/mcpInstPol.json
/api/class/fabricNodeBlk.json
/api/class/faultInst.json
/api/class/eqptcapacityL3TotalUsageCap5min.json
/api/class/fvcapRule.json
/api/class/pkiExportEncryptionKey.json
```

# Security
This tool only collects the output of the afformentioned managed objects. Documentation on these endpoints is available in the [full API documentation](https://developer.cisco.com/site/apic-mim-ref-api/). Credentials are only used at the point of collection and are not stored in any way.

All data provided to Cisco will be maintained under Cisco's data retention policy.

# Usage

All command line paramters are optional; the tool will prompt for any missing information. This is a command line tool, but can be run directly from the Windows/Mac/Linux GUI if desired--the tool will pause once complete, before closing the terminal.

```
Usage: aci-vetr-c [--ip IP] [--username USERNAME] [--password PASSWORD] [--output OUTPUT] [--debug]

Options:
  --ip IP, -i IP         APIC IP address
  --username USERNAME, -u USERNAME
                         APIC username
  --password PASSWORD, -p PASSWORD
                         APIC password
  --output OUTPUT, -o OUTPUT
                         Output file [default: aci-vetr-data.zip]
  --debug, -d            Debug output
  --help, -h             display this help and exit
  --version              display version and exit
```
