package main

import "github.com/brightpuddle/goaci"

// Request is an HTTP request.
type Request struct {
	req    goaci.Req // goACI request object
	prefix string    // Prefix for the DB
}

// Create new request and use classname as db prefix
func newRequest(class string, mods ...func(*goaci.Req)) Request {
	req := goaci.NewReq("GET", "api/class/"+class, nil, mods...)
	return Request{req: req, prefix: class}
}

var reqs = []Request{
	/************************************************************
	Infrastructure
	************************************************************/
	newRequest("topSystem"),    // All devices
	newRequest("eqptBoard"),    // APIC hardware
	newRequest("fabricNode"),   // Switch hardware
	newRequest("fabricSetupP"), // Pods (fabric setup policy)

	/************************************************************
	Fabric-wide settings
	************************************************************/
	newRequest("epLoopProtectP"),    // EP loop protection policy
	newRequest("epControlP"),        // Rogue EP control policy
	newRequest("epIpAgingP"),        // IP aging policy
	newRequest("infraSetPol"),       // Fabric-wide settings
	newRequest("infraPortTrackPol"), // Port tracking policy
	newRequest("coopPol"),           // COOP group policy

	/************************************************************
	Tenants
	************************************************************/
	// Primary constructs
	newRequest("fvAEPg"),   // EPG
	newRequest("fvRsBd"),   // EPG --> BD
	newRequest("fvBD"),     // BD
	newRequest("fvCtx"),    // VRF
	newRequest("fvTenant"), // Tenant
	newRequest("fvSubnet"), // Subnet

	// Contracts
	newRequest("vzBrCP"),          // Contract
	newRequest("vzFilter"),        // Filter
	newRequest("vzSubj"),          // Subject
	newRequest("vzRsSubjFiltAtt"), // Subject --> filter
	newRequest("fvRsProv"),        // EPG --> contract provided
	newRequest("fvRsCons"),        // EPG --> contract consumed

	// L3outs
	newRequest("l3extOut"),            // L3out
	newRequest("l3extLNodeP"),         // L3 node profile
	newRequest("l3extRsNodeL3OutAtt"), // Node profile --> Node
	newRequest("l3extLIfP"),           // L3 interface profile
	newRequest("l3extInstP"),          // External EPG

	/************************************************************
	Fabric Policies
	************************************************************/
	newRequest("isisDomPol"),         // ISIS policy
	newRequest("bgpRRNodePEp"),       // BGP route reflector nodes
	newRequest("l3IfPol"),            // L3 interface policy
	newRequest("fabricNodeControl"),  // node control (Dom, netflow,etc)
	newRequest("fabricRsNodeCtrl"),   // node policy group --> node control
	newRequest("fabricRsLeNodePGrp"), // leaf --> leaf node policy group
	newRequest("fabricNodeBlk"),      // Node block

	/************************************************************
	Fabric Access
	************************************************************/
	// MCP
	newRequest("mcpIfPol"),          // MCP inteface policy
	newRequest("infraRsMcpIfPol"),   // MCP pol --> policy group
	newRequest("infraRsAccBaseGrp"), // policy group --> host port selector
	newRequest("infraRsAccPortP"),   // int profile --> node profile

	newRequest("mcpInstPol"), // MCP global policy

	// AEP/domain/VLANs
	newRequest("infraAttEntityP"), // AEP
	newRequest("infraRsDomP"),     // AEP --> domain
	newRequest("infraRsVlanNs"),   // Domain --> VLAN pool
	newRequest("fvnsEncapBlk"),    // VLAN encap block

	/************************************************************
	Admin/Operations
	************************************************************/
	newRequest("firmwareRunning"),        // Switch firmware
	newRequest("firmwareCtrlrRunning"),   // Controller firmware
	newRequest("pkiExportEncryptionKey"), // Crypto key

	/************************************************************
	Live State
	************************************************************/
	newRequest("faultInst"), // Faults
	newRequest("fvcapRule"), // Capacity rules
	// Endpoint count
	newRequest("fvCEp", goaci.Query("rsp-subtree-include", "count")),
	// IP count
	newRequest("fvIp", goaci.Query("rsp-subtree-include", "count")),
	// L4-L7 container count
	newRequest("vnsCDev", goaci.Query("rsp-subtree-include", "count")),
	// L4-L7 service graph count
	newRequest("vnsGraphInst", goaci.Query("rsp-subtree-include", "count")),
	// MO count by node
	newRequest("ctxClassCnt", goaci.Query("rsp-subtree-class", "l2BD,fvEpP,l3Dom")),

	// Fabric health
	newRequest("fabricHealthTotal"), // Total and per-pod health scores
	{ // Per-device health stats
		req:    newRequest("topSystem", goaci.Query("rsp-subtree-include", "health,no-scoped")).req,
		prefix: "healthInst",
	},

	// Switch capacity
	newRequest("eqptcapacityVlanUsage5min"),        // VLAN
	newRequest("eqptcapacityPolUsage5min"),         // TCAM
	newRequest("eqptcapacityL2Usage5min"),          // L2 local
	newRequest("eqptcapacityL2RemoteUsage5min"),    // L2 remote
	newRequest("eqptcapacityL2TotalUsage5min"),     // L2 total
	newRequest("eqptcapacityL3Usage5min"),          // L3 local
	newRequest("eqptcapacityL3UsageCap5min"),       // L3 local cap
	newRequest("eqptcapacityL3RemoteUsage5min"),    // L3 remote
	newRequest("eqptcapacityL3RemoteUsageCap5min"), // L3 remote cap
	newRequest("eqptcapacityL3TotalUsage5min"),     // L3 total
	newRequest("eqptcapacityL3TotalUsageCap5min"),  // L3 total cap
	newRequest("eqptcapacityMcastUsage5min"),       // Multicast
}
