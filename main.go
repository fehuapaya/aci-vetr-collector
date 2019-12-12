package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/brightpuddle/goaci"
	"github.com/mattn/go-colorable"
	"github.com/mholt/archiver"
	"github.com/rs/zerolog"
	"github.com/tidwall/buntdb"
)

// Version comes from CI
var version string

const (
	schemaVersion = 23
	resultZip     = "aci-vetr-data.zip"
	logFile       = "aci-vetr-c.log"
	dbName        = "data.db"
)

var wg sync.WaitGroup

var log zerolog.Logger

// Args : command line parameters
type Args struct {
	IP       string `arg:"-i" help:"APIC IP address"`
	Username string `arg:"-u" help:"APIC username"`
	Password string `arg:"-p" help:"APIC password"`
	Output   string `arg:"-o" help:"Output file"`
	Debug    bool   `arg:"-d" help:"Debug output"`
}

// Description is the CLI description string.
func (Args) Description() string {
	return "ACI vetR collector"
}

// Version is the CLI version string.
func (Args) Version() string {
	return fmt.Sprintf("version %s\nschema version %d", version, schemaVersion)
}

func newArgsFromCLI() Args {
	args := Args{Output: resultZip}
	arg.MustParse(&args)
	return args
}

func runningTime(msg string) (string, time.Time) {
	startTime := time.Now()
	log.Debug().Time("start_time", startTime).Msgf("begin: %s", msg)
	return msg, startTime
}

func elapsed(msg string, startTime time.Time) {
	log.Debug().
		TimeDiff("elapsed_time", time.Now(), startTime).
		Msgf("done: %s", msg)
}

// HTTP queries
type Query struct {
	key   string
	value string
}

// Request is a customized wrapper for goaci.Req
type Request struct {
	name     string             // Custom class name for DB - use class by default
	class    string             // MO class to query
	queries  []func(*goaci.Req) // query paramters
	filter   string             // GJSON path query for the result
	optional bool               // Fail on unsuccessful collection?
}

var reqs = []Request{
	/************************************************************
	Infrastructure
	************************************************************/
	{class: "topSystem"},    // All devicdes
	{class: "eqptBoard"},    // APIC hardware
	{class: "fabricNode"},   // Switch hardware
	{class: "fabricSetupP"}, // Pods (fabric setup policy)

	/************************************************************
	Fabric-wide settings
	************************************************************/
	{class: "epLoopProtectP"},    // EP loop protection policy
	{class: "epControlP"},        // Rogue EP control policy
	{class: "epIpAgingP"},        // IP aging policy
	{class: "infraSetPol"},       // Fabric-wide settings
	{class: "infraPortTrackPol"}, // Port tracking policy
	{class: "coopPol"},           // COOP group policy

	/************************************************************
	Tenants
	************************************************************/
	// Primary constructs
	{class: "fvAEPg"},   // EPG
	{class: "fvRsBd"},   // EPG --> BD
	{class: "fvBD"},     // BD
	{class: "fvCtx"},    // VRF
	{class: "fvTenant"}, // Tenant
	{class: "fvSubnet"}, // Subnet

	// Contracts
	{class: "vzBrCP"},          // Contract
	{class: "vzFilter"},        // Filter
	{class: "vzSubj"},          // Subject
	{class: "vzRsSubjFiltAtt"}, // Subject --> filter
	{class: "fvRsProv"},        // EPG --> contract provided
	{class: "fvRsCons"},        // EPG --> contract consumed

	// L3outs
	{class: "l3extOut"},            // L3out
	{class: "l3extLNodeP"},         // L3 node profile
	{class: "l3extRsNodeL3OutAtt"}, // Node profile --> Node
	{class: "l3extLIfP"},           // L3 interface profile
	{class: "l3extInstP"},          // External EPG

	/************************************************************
	Fabric Policies
	************************************************************/
	{class: "isisDomPol"},         // ISIS policy
	{class: "bgpRRNodePEp"},       // BGP route reflector nodes
	{class: "l3IfPol"},            // L3 interface policy
	{class: "fabricNodeControl"},  // node control (Dom, netflow,etc)
	{class: "fabricRsNodeCtrl"},   // node policy group --> node control
	{class: "fabricRsLeNodePGrp"}, // leaf --> leaf node policy group
	{class: "fabricNodeBlk"},      // Node block

	/************************************************************
	Fabric Access
	************************************************************/
	// MCP
	{class: "mcpIfPol"},          // MCP inteface policy
	{class: "infraRsMcpIfPol"},   // MCP pol --> policy group
	{class: "infraRsAccBaseGrp"}, // policy group --> host port selector
	{class: "infraRsAccPortP"},   // int profile --> node profile

	{class: "mcpInstPol"}, // MCP global policy

	// AEP/domain/VLANs
	{class: "infraAttEntityP"}, // AEP
	{class: "infraRsDomP"},     // AEP --> domain
	{class: "infraRsVlanNs"},   // Domain --> VLAN pool
	{class: "fvnsEncapBlk"},    // VLAN encap block

	/************************************************************
	Admin/Operations
	************************************************************/
	{class: "firmwareRunning"},        // Switch firmware
	{class: "firmwareCtrlrRunning"},   // Controller firmware
	{class: "pkiExportEncryptionKey"}, // Crypto key

	/************************************************************
	Live State
	************************************************************/
	{class: "faultInst"}, // Faults
	{class: "fvcapRule"}, // Capacity rules
	{ // Endpoint count
		class:   "fvCEp",
		queries: []func(*goaci.Req){goaci.Query("rsp-subtree-include", "count")},
		filter:  "#.moCount.attributes",
	},
	{ // IP count
		class:   "fvIp",
		queries: []func(*goaci.Req){goaci.Query("rsp-subtree-include", "count")},
		filter:  "#.moCount.attributes",
	},
	{ // L4-L7 container count
		class:   "vnsCDev",
		queries: []func(*goaci.Req){goaci.Query("rsp-subtree-include", "count")},
		filter:  "#.moCount.attributes",
	},
	{ // L4-L7 service graph count
		class:   "vnsGraphInst",
		queries: []func(*goaci.Req){goaci.Query("rsp-subtree-include", "count")},
		filter:  "#.moCount.attributes",
	},
	{ // MO count by node
		class:   "ctxClassCnt",
		queries: []func(*goaci.Req){goaci.Query("rsp-subtree-class", "l2BD,fvEpP,l3Dom")},
	},

	// Fabric health
	{class: "fabricHealthTotal"}, // Total and per-pod health scores
	{ // Per-device health stats
		name:    "healthInst",
		class:   "topSystem",
		queries: []func(j *goaci.Req){goaci.Query("rsp-subtree-include", "health,no-scoped")},
	},

	// Switch capacity
	{class: "eqptcapacityVlanUsage5min"},                        // VLAN
	{class: "eqptcapacityPolUsage5min"},                         // TCAM
	{class: "eqptcapacityL2Usage5min"},                          // L2 local
	{class: "eqptcapacityL2RemoteUsage5min", optional: true},    // L2 remote
	{class: "eqptcapacityL2TotalUsage5min", optional: true},     // L2 total
	{class: "eqptcapacityL3Usage5min"},                          // L3 local
	{class: "eqptcapacityL3UsageCap5min"},                       // L3 local cap
	{class: "eqptcapacityL3RemoteUsage5min", optional: true},    // L3 remote
	{class: "eqptcapacityL3RemoteUsageCap5min", optional: true}, // L3 remote cap
	{class: "eqptcapacityL3TotalUsage5min", optional: true},     // L3 total
	{class: "eqptcapacityL3TotalUsageCap5min", optional: true},  // L3 total cap
	{class: "eqptcapacityMcastUsage5min"},                       // Multicast
}

func fetch(client goaci.Client, req Request, db *buntdb.DB) {
	defer elapsed(runningTime(req.class))
	log.Info().Str("class", req.class).Msg("fetching resource...")
	uri := fmt.Sprintf("/api/class/%s", req.class)
	log.Debug().
		Str("uri", uri).
		Interface("query", req.queries).
		Msg("requesting resource")
	res, err := client.GetClass(req.class, req.queries...)
	if err != nil && !req.optional {
		log.Panic().
			Err(err).
			Str("class", req.class).
			Interface("query", req.queries).
			Msg("Failed to make request. Please report this error to Cisco.")
	}
	if req.name == "" {
		req.name = req.class
	}
	if req.filter == "" {
		req.filter = fmt.Sprintf("#.%s.attributes", req.name)
	}
	if err := db.Update(func(tx *buntdb.Tx) error {
		for _, record := range res.Get(req.filter).Array() {
			log.Debug().
				Str("prefix", req.name).
				Str("dn", record.Get("dn").Str).
				Msg("set_db")
			key := fmt.Sprintf("%s:%s", req.name, record.Get("dn").Str)
			if _, _, err := tx.Set(key, record.Raw, nil); err != nil {
				log.Panic().Err(err).Msg("cannot set key")
			}
		}
		return nil
	}); err != nil {
		log.Panic().Err(err).Msg("cannot write to db file")
	}

	wg.Done()
}

func initLogger(args Args) zerolog.Logger {
	file, err := os.Create(logFile)
	if err != nil {
		panic(fmt.Sprintf("cannot create log file %s", logFile))
	}

	if !args.Debug {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	zerolog.DurationFieldInteger = true

	return zerolog.New(zerolog.MultiLevelWriter(
		file,
		zerolog.ConsoleWriter{Out: colorable.NewColorableStdout()},
	)).With().Timestamp().Logger()
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msg("Collection failed.")
		}
		os.Remove(dbName)
		os.Remove(logFile)
		fmt.Println("Press enter to exit.")
		var throwaway string
		fmt.Scanln(&throwaway)
	}()

	// Get config
	args := newArgsFromCLI()
	log = initLogger(args)
	client, err := goaci.NewClient(
		args.IP,
		args.Username,
		args.Password,
		goaci.RequestTimeout(600),
	)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to create ACI client.")
	}

	// Authenticate
	log.Info().Str("host", args.IP).Msg("APIC host")
	log.Info().Str("user", args.Username).Msg("APIC username")
	log.Info().Msg("Authenticating to the APIC...")
	if err := client.Login(); err != nil {
		log.Panic().
			Err(err).
			Str("ip", args.IP).
			Str("user", args.Username).
			Msg("cannot authenticate to the APIC.")
	}

	db, err := buntdb.Open(dbName)
	if err != nil {
		log.Panic().Err(err).Str("file", dbName).Msg("cannot open output file")
	}

	// Fetch data from API
	fmt.Println(strings.Repeat("=", 30))
	for _, req := range reqs {
		wg.Add(1)
		go fetch(client, req, db)
	}
	wg.Wait()

	fmt.Println(strings.Repeat("=", 30))

	// Add metadata
	metadata, _ := json.Marshal(map[string]interface{}{
		"collectorVersion": version,
		"schemaVersion":    schemaVersion,
		"timestamp":        time.Now(),
	})
	if err := db.Update(func(tx *buntdb.Tx) error {
		if _, _, err := tx.Set("meta", string(metadata), nil); err != nil {
			log.Panic().Err(err).Msg("cannot write metadata to db")
		}
		return nil
	}); err != nil {
		log.Panic().Err(err).Msg("cannot update db file")
	}

	db.Close()

	// Create archive
	log.Info().Msg("Creating archive")
	os.Remove(args.Output) // Remove any old archives and ignore errors
	if err := archiver.Archive([]string{dbName, logFile}, args.Output); err != nil {
		log.Panic().
			Err(err).
			Str("src", dbName).
			Str("dst", args.Output).
			Msg("cannot create archive")
	}

	// Cleanup
	fmt.Println(strings.Repeat("=", 30))
	log.Info().Msg("Collection complete.")
	log.Info().Msgf("Please provide %s to Cisco Services for further analysis.",
		args.Output)
}
