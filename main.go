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

// Args are command line parameters.
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

// Request is an HTTP request.
type Request struct {
	req    goaci.Req // goACI request object
	prefix string    // Prefix for the DB
}

// Create new request and use classname as db prefix
func newRequest(class string, mods ...func(*goaci.Req)) Request {
	req := goaci.NewReq("GET", "api/class/"+class, nil, mods...)
	return Request{req, class}
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
		newRequest("topSystem", goaci.Query("rsp-subtree-include", "health,no-scoped")).req,
		"healthInst",
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

func fetch(client goaci.Client, req Request, db *buntdb.DB) {
	defer elapsed(runningTime(req.prefix))
	log.Info().Str("class", req.prefix).Msg("fetching resource...")
	log.Debug().
		Str("url", req.req.HttpReq.URL.String()).
		Msg("requesting resource")
	res, err := client.Do(req.req)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", req.req.HttpReq.URL.String()).
			Msg("failed to make request")
	}
	if err := db.Update(func(tx *buntdb.Tx) error {
		for _, record := range res.Get("imdata.#.*.attributes").Array() {
			dn := record.Get("dn").Str
			if dn == "" {
				log.Panic().Str("record", record.Raw).Msg("DN empty")
			}
			log.Debug().
				Interface("req", req).
				Str("dn", dn).
				Msg("set_db")
			key := fmt.Sprintf("%s:%s", req.prefix, record.Get("dn").Str)
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
