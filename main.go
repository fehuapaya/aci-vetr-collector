package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aci-vetr-c/aci"

	"github.com/alexflint/go-arg"
	"github.com/mholt/archiver"
	"github.com/rs/zerolog"
	"github.com/tidwall/buntdb"
)

// Version comes from CI
var version string

const (
	schemaVersion = 20
	resultZip     = "aci-vetr-data.zip"
	logFile       = "aci-vetr-c.log"
	dbName        = "data.db"
)

var wg sync.WaitGroup

var log zerolog.Logger
var out zerolog.Logger

// Config : command line parameters
type Config struct {
	IP       string `arg:"-i" help:"APIC IP address"`
	Username string `arg:"-u"`
	Password string `arg:"-p"`
	Output   string `arg:"-o"`
}

// Description : CLI description string
func (Config) Description() string {
	return "ACI vetR collector"
}

// Version : CLI version string
func (Config) Version() string {
	return fmt.Sprintf("version %s\nschema version %d", version, schemaVersion)
}

func newConfigFromCLI() Config {
	cfg := Config{Output: resultZip}
	arg.MustParse(&cfg)
	return cfg
}

type request struct {
	class    string
	query    []string
	filter   string
	optional bool
}

var reqs = []request{
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
	{class: "fabricNodeControl"},  // node control (Dom, netflow,etc)
	{class: "fabricRsNodeCtrl"},   // node policy group --> node control
	{class: "fabricRsLeNodePGrp"}, // leaf --> leaf node policy group
	{class: "fabricNodeBlk"},      // Node block

	/************************************************************
	Fabric Access
	************************************************************/
	// MCP
	{class: "mcpIfPol"},   // MCP inteface policy
	{class: "mcpInstPol"}, // MCP global policy

	// AEP/domain/VLANs
	{class: "infraAttEntityP"}, // AEP
	{class: "infraRsDomP"},     // AEP --> domain
	{class: "infraRsVlanNs"},   // Domain --> VLAN pool
	{class: "fvnsEncapBlk"},    // VLAN encap block

	/************************************************************
	Admin/Operations
	************************************************************/
	{class: "firmwareRunning"},      // Switch firmware
	{class: "firmwareCtrlrRunning"}, // Controller firmware
	// TODO Firmware groups
	// TODO Backup settings

	{class: "pkiExportEncryptionKey"}, // Crypto key

	/************************************************************
	Live State
	************************************************************/
	{class: "faultInst"}, // Faults
	{class: "fvcapRule"}, // Capacity rules
	{ // Endpoint count
		class:  "fvCEp",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{ // IP count
		class:  "fvIp",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{ // L4-L7 container count
		class:  "vnsCDev",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{ // L4-L7 service graph count
		class:  "vnsGraphInst",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{ // MO count by node
		class: "ctxClassCnt",
		query: []string{"rsp-subtree-class=l2BD,fvEpP,l3Dom"},
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

func fetch(client aci.Client, req request, db *buntdb.DB) {
	fmt.Printf("fetching %s\n", req.class)
	log.Debug().
		Dict("request", zerolog.Dict().
			Str("class", req.class).
			Interface("query", req.query)).
		Msg("requesting resource")
	res, err := client.Get(aci.Req{
		URI:   fmt.Sprintf("/api/class/%s", req.class),
		Query: req.query,
	})
	if err != nil && !req.optional {
		fmt.Println("Please report the following error:")
		fmt.Printf("%+v\n", req)
		out.Panic().
			Err(err).
			Dict("request", zerolog.Dict().
				Str("class", req.class).
				Interface("query", req.query)).
			Msg("failed to make request")
	}

	if err := db.Update(func(tx *buntdb.Tx) error {
		if req.filter == "" {
			req.filter = fmt.Sprintf("#.%s.attributes", req.class)
		}
		for _, record := range res.Get(req.filter).Array() {
			key := fmt.Sprintf("%s:%s", req.class, record.Get("dn").Str)
			if _, _, err := tx.Set(key, record.Raw, nil); err != nil {
				out.Panic().Err(err).Msg("cannot set key")
			}
		}
		return nil
	}); err != nil {
		out.Panic().Err(err).Msg("cannot write to db file")
	}

	wg.Done()
}

func init() {
	// Setup logger
	file, err := os.Create(logFile)
	if err != nil {
		panic(fmt.Sprintf("cannot create log file %s", logFile))
	}

	log = zerolog.New(file).With().Timestamp().Logger()
	out = zerolog.New(zerolog.MultiLevelWriter(
		file,
		zerolog.ConsoleWriter{Out: os.Stderr, NoColor: true},
	)).With().Timestamp().Logger()
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Collection unsuccessfully.")
		}
		fmt.Println("Press enter to exit.")
		var throwaway string
		fmt.Scanln(&throwaway)
	}()

	// Get config
	cfg := newConfigFromCLI()
	client := aci.NewClient(aci.Config{
		IP:             cfg.IP,
		Username:       cfg.Username,
		Password:       cfg.Password,
		RequestTimeout: 600,
	})

	// Authenticate
	fmt.Println("\nauthenticating to the APIC")
	if err := client.Login(); err != nil {
		out.Panic().
			Err(err).
			Str("ip", cfg.IP).
			Str("user", cfg.Username).
			Msg("cannot authenticate to the APIC")
	}

	db, err := buntdb.Open(dbName)
	if err != nil {
		out.Panic().Err(err).Str("file", dbName).Msg("cannot open output file")
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
			out.Panic().Err(err).Msg("cannot write metadata to db")
		}
		return nil
	}); err != nil {
		out.Panic().Err(err).Msg("cannot update db file")
	}

	if err := db.Shrink(); err != nil {
		out.Panic().Err(err).Msg("cannot shrink db file")
	}
	db.Close()

	// Create archive
	fmt.Println("creating archive")
	os.Remove(resultZip) // Remove any old archives and ignore errors
	if err := archiver.Archive([]string{dbName, logFile}, resultZip); err != nil {
		out.Panic().
			Err(err).
			Str("src", dbName).
			Str("dst", resultZip).
			Msg("cannot create archive")
	}

	// Cleanup
	os.Remove(dbName)
	os.Remove(logFile)
	fmt.Println(strings.Repeat("=", 30))
	fmt.Println("Collection complete.")
	fmt.Printf("Please provide %s to Cisco services for further analysis.\n",
		resultZip)
}
