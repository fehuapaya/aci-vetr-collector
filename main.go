package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/brightpuddle/go-aci"
	"github.com/mholt/archiver"
	"github.com/rs/zerolog"
	"github.com/tidwall/buntdb"
)

const (
	schemaVersion = 16
	version       = "1.2.0"
	resultZip     = "aci-vet-data.zip"
	logFile       = "aci-vet.log"
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
}

// Description : CLI description string
func (Config) Description() string {
	return "ACI Collector"
}

// Version : CLI version string
func (Config) Version() string {
	return fmt.Sprintf("version %s\nschema version %d", version, schemaVersion)
}

func newConfigFromCLI() (cfg Config) {
	arg.MustParse(&cfg)
	return
}

type request struct {
	name     string
	class    string
	query    []string
	filter   string
	optional bool
}

var reqs = []request{
	/************************************************************
	Infrastructure
	************************************************************/
	{name: "hardware-apic", class: "eqptBoard"},
	{name: "devices", class: "topSystem"},
	{name: "pods", class: "fabricSetupP"},
	{name: "hardware-switch", class: "fabricNode"},

	/************************************************************
	Fabric-wide settings
	************************************************************/
	// Endpoint Controls
	{name: "ep-loop-control", class: "epLoopProtectP"},
	{name: "rogue-ep-control", class: "epControlP"},
	{name: "ip-aging", class: "epIpAgingP"},

	// Fabric-Wide Settings
	{name: "fabric-wide-settings", class: "infraSetPol"},

	// Port-tracking
	{name: "port-tracking", class: "infraPortTrackPol"},

	/************************************************************
	Tenants
	************************************************************/
	// Primary constructs
	{name: "epg", class: "fvAEPg"},
	{name: "epg-bd-association", class: "fvRsBd"},
	{name: "bd", class: "fvBD"},
	{name: "vrf", class: "fvCtx"},
	{name: "tenant", class: "fvTenant"},
	{name: "subnet", class: "fvSubnet"},

	// Contracts
	{name: "contract", class: "vzBrCP"},
	{name: "subject", class: "vzSubj"},
	{name: "filter", class: "vzRsSubjFiltAtt"},
	{name: "contract-consumed", class: "fvRsProv"},
	{name: "contract-consumed", class: "fvRsCons"},

	// L3outs
	{name: "ext-epg", class: "l3extInstP"},
	{name: "l3out", class: "l3extOut"},
	{name: "l3-int-profile", class: "l3extLIfP"},
	{name: "l3-node-profile", class: "l3extLNodeP"},

	/************************************************************
	Fabric Policies
	************************************************************/
	{name: "isis-policy", class: "isisDomPol"},
	{name: "bgp-route-reflector", class: "bgpRRNodePEp"},
	{name: "node-control-policy", class: "fabricNodeControl"},
	{name: "fabric-leaf-policy-group", class: "fabricLeNodePGrp"},
	{name: "fabric-leaf-policy-association", class: "fabricRsNodeCtrl"},
	{name: "fabric-leaf-profile", class: "fabricLeafP"},
	{name: "fabric-leaf-switch-association", class: "fabricLeafS"},
	{name: "node-block", class: "fabricNodeBlk"},

	/************************************************************
	Fabric Access
	************************************************************/
	// Interface policy
	{name: "mcp-interface-policy", class: "mcpIfPol"},

	// Global policy
	{name: "mcp-global-policy", class: "mcpInstPol"},

	// AEP/domain/VLANs
	{name: "aep", class: "infraAttEntityP"},
	{name: "aep-domain-association", class: "infraRsDomP"},
	{name: "domain-vlan-association", class: "infraRsVlanNs"},
	{name: "vlan-pool", class: "fvnsEncapBlk"},

	/************************************************************
	Admin/Operations
	************************************************************/
	{name: "firmware-switch", class: "firmwareRunning"},
	{name: "firmware-controller", class: "firmwareCtrlrRunning"},
	// TODO Firmware groups
	// TODO Backup settings

	{name: "crypto-key", class: "pkiExportEncryptionKey"},

	/************************************************************
	Live State
	************************************************************/
	{name: "fault", class: "faultInst"},
	{name: "capacity-rule", class: "fvcapRule"},
	{
		name:   "ep-count",
		class:  "fvEpP",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{
		name:   "ip-count",
		class:  "fvIp",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{
		name:   "l4l7-container-count",
		class:  "vnsCDev",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{
		name:   "l4l7-service-graph-count",
		class:  "vnsGraphInst",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	{
		name:  "mo-count-by-node",
		class: "ctxClassCnt",
		query: []string{"rsp-subtree-class=l2BD,fvEpP,l3Dom"},
	},
	{name: "capacity-vlan", class: "eqptcapacityVlanUsage5min"},
	{name: "capacity-tcam", class: "eqptcapacityPolUsage5min"},
	{name: "capacity-l2-local", class: "eqptcapacityL2Usage5min"},
	{
		name:     "capacity-l2-remote",
		class:    "eqptcapacityL2RemoteUsage5min",
		optional: true,
	},
	{
		name:     "capacity-l2-total",
		class:    "eqptcapacityL2TotalUsage5min",
		optional: true,
	},
	{name: "capacity-l3-local", class: "eqptcapacityL3Usage5min"},
	{
		name:     "capacity-l3-remote",
		class:    "eqptcapacityL3RemoteUsage5min",
		optional: true,
	},
	{
		name:     "capacity-l3-total",
		class:    "eqptcapacityL3TotalUsage5min",
		optional: true,
	},
	{name: "capacity-l3-local-cap", class: "eqptcapacityL3UsageCap5min"},
	{
		name:     "capacity-l3-remote-cap",
		class:    "eqptcapacityL3RemoteUsageCap5min",
		optional: true,
	},
	{
		name:     "capacity-l3-total-cap",
		class:    "eqptcapacityL3TotalUsageCap5min",
		optional: true,
	},
	{name: "capacity-mcast", class: "eqptcapacityMcastUsage5min"},
}

func fetch(client aci.Client, req request, db *buntdb.DB) {
	fmt.Printf("fetching %s\n", req.name)
	log.Debug().
		Dict("request", zerolog.Dict().
			Str("name", req.name).
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
		out.Fatal().
			Err(err).
			Dict("request", zerolog.Dict().
				Str("name", req.name).
				Str("class", req.class).
				Interface("query", req.query)).
			Msg("failed to make request")
	}

	db.Update(func(tx *buntdb.Tx) error {
		if req.filter == "" {
			req.filter = fmt.Sprintf("#.%s.attributes", req.class)
		}
		for _, record := range res.Get(req.filter).Array() {
			key := fmt.Sprintf("%s:%s", req.name, record.Get("dn").Str)
			tx.Set(key, record.Raw, nil)
		}
		return nil
	})

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
	// Get config
	cfg := newConfigFromCLI()
	client := aci.NewClient(aci.Config{
		IP:             cfg.IP,
		Username:       cfg.Username,
		Password:       cfg.Password,
		RequestTimeout: 90,
	})

	// Authenticate
	fmt.Println("\nauthenticating to the APIC")
	if err := client.Login(); err != nil {
		out.Fatal().
			Err(err).
			Str("ip", cfg.IP).
			Str("user", cfg.Username).
			Msg("cannot authenticate to the APIC")
	}

	db, err := buntdb.Open(dbName)
	if err != nil {
		out.Fatal().Err(err).Str("file", dbName).Msg("cannot open output file")
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
	db.Update(func(tx *buntdb.Tx) error {
		tx.Set("meta", string(metadata), nil)
		return nil
	})

	db.Shrink()
	db.Close()

	// Create archive
	fmt.Println("creating archive")
	os.Remove(resultZip) // Remove any old archives and ignore errors
	if err := archiver.Archive([]string{dbName, logFile}, resultZip); err != nil {
		out.Fatal().
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
