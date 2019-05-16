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
	schemaVersion = 7
	version       = "0.2.2"
	resultZip     = "health-check-data.zip"
	logFile       = "aci-collector.log"
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
	return "ACI health check collector"
}

// Version : CLI version string
func (Config) Version() string {
	return fmt.Sprintf("version %s", version)
}

func newConfigFromCLI() (cfg Config) {
	arg.MustParse(&cfg)
	return
}

type request struct {
	name   string
	class  string
	query  []string
	filter string
}

var reqs = []request{

	// Tenant objects
	request{
		name:   "bds",
		class:  "fvBD",
		filter: "#.fvBD.attributes",
	},
	request{
		name:   "contracts",
		class:  "vzBrCP",
		filter: "#.vzBrCP.attributes",
	},
	request{
		name:   "epgs",
		class:  "fvEpP",
		filter: "#.fvEpP.attributes",
	},
	request{
		name:   "ext-epgs",
		class:  "l3extInstP",
		filter: "#.l3extInstP.attributes",
	},
	request{
		name:   "filters",
		class:  "vzRsSubjFiltAtt",
		filter: "#.vzRsSubjFiltAtt.attributes",
	},
	request{
		name:   "l3-int-profiles",
		class:  "l3extLIfP",
		filter: "#.l3extLIfP.attributes",
	},
	request{
		name:   "l3-node-profiles",
		class:  "l3extLNodeP",
		filter: "#.l3extLNodeP.attributes",
	},
	request{
		name:   "l3outs",
		class:  "l3extOut",
		filter: "#.l3extOut.attributes",
	},
	request{
		name:   "subjects",
		class:  "vzSubj",
		filter: "#.vzSubj.attributes",
	},
	request{
		name:   "tenants",
		class:  "fvTenant",
		filter: "#.fvTenant.attributes",
	},
	request{
		name:   "vrfs",
		class:  "fvCtx",
		filter: "#.fvCtx.attributes",
	},

	// Infrastructure
	request{
		name:   "hardware-apic",
		class:  "eqptBoard",
		filter: "#.eqptBoard.attributes",
	},
	request{
		name:   "devices",
		class:  "topSystem",
		filter: "#.topSystem.attributes",
	},
	request{
		name:   "pods",
		class:  "fabricSetupP",
		filter: "#.fabricSetupP.attributes",
	},
	request{
		name:   "hardware-switch",
		class:  "fabricNode",
		filter: "#.fabricNode.attributes",
	},

	// State
	request{
		name:   "faults",
		class:  "faultInfo",
		filter: "#.faultInst.attributes",
	},
	request{
		name:   "capacity-rules",
		class:  "fvcapRule",
		filter: "#.fvcapRule.attributes",
	},
	request{
		name:   "ep-count",
		class:  "fvEpP",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	request{
		name:   "ip-count",
		class:  "fvIp",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	request{
		name:   "l4l7-container-count",
		class:  "vnsCDev",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	request{
		name:   "l4l7-service-graph-count",
		class:  "vnsGraphInst",
		query:  []string{"rsp-subtree-include=count"},
		filter: "#.moCount.attributes",
	},
	request{
		name:   "mo-count-by-node",
		class:  "ctxClassCnt",
		query:  []string{"rsp-subtree-class=l2BD,fvEpP,l3Dom"},
		filter: "#.ctxClassCnt.attributes",
	},
	request{
		name:   "capacity-vlan",
		class:  "eqptcapacityVlanUsage5min",
		filter: "#.eqptcapacityVlanUsage5min.attributes",
	},
	request{
		name:   "capacity-tcam",
		class:  "eqptcapacityPolUsage5min",
		filter: "#.eqptcapacityPolUsage5min.attributes",
	},
	request{
		name:   "capacity-l2-local",
		class:  "eqptcapacityL2Usage5min",
		filter: "#.eqptcapacityL2Usage5min.attributes",
	},
	request{
		name:   "capacity-l2-remote",
		class:  "eqptcapacityL2RemoteUsage5min",
		filter: "#.eqptcapacityL2RemoteUsage5min.attributes",
	},
	request{
		name:   "capacity-l2-total",
		class:  "eqptcapacityL2TotalUsage5min",
		filter: "#.eqptcapacityL2TotalUsage5min.attributes",
	},
	request{
		name:   "capacity-l3-local",
		class:  "eqptcapacityL3Usage5min",
		filter: "#.eqptcapacityL3Usage5min.attributes",
	},
	request{
		name:   "capacity-l3-remote",
		class:  "eqptcapacityL3RemoteUsage5min",
		filter: "#.eqptcapacityL3RemoteUsage5min.attributes",
	},
	request{
		name:   "capacity-l3-total",
		class:  "eqptcapacityL3TotalUsage5min",
		filter: "#.eqptcapacityL3TotalUsage5min.attributes",
	},
	request{
		name:   "capacity-l3-local-cap",
		class:  "eqptcapacityL3UsageCap5min",
		filter: "#.eqptcapacityL3UsageCap5min.attributes",
	},
	request{
		name:   "capacity-l3-remote-cap",
		class:  "eqptcapacityL3RemoteUsageCap5min",
		filter: "#.eqptcapacityL3RemoteUsageCap5min.attributes",
	},
	request{
		name:   "capacity-l3-total-cap",
		class:  "eqptcapacityL3TotalUsageCap5min",
		filter: "#.eqptcapacityL3TotalUsageCap5min.attributes",
	},
	request{
		name:   "capacity-mcast",
		class:  "eqptcapacityMcastUsage5min",
		filter: "#.eqptcapacityMcastUsage5min.attributes",
	},

	// Global config
	request{
		name:   "bgp-route-reflectors",
		class:  "bgpRRNodePEp",
		filter: "#.bgpRRNodePEp.attributes",
	},
	request{
		name:   "crypto-key",
		class:  "pkiExportEncryptionKey",
		filter: "#.pkiExportEncryptionKey.attributes",
	},
	request{
		name:   "ep-loop-control",
		class:  "epLoopProtectP",
		filter: "#.epLoopProtectP.attributes",
	},
	request{
		name:   "fabric-wide-settings",
		class:  "infraSetPol",
		filter: "#.infraSetPol.attributes",
	},
	request{
		name:   "ip-aging",
		class:  "epIpAgingP",
		filter: "#.epIpAgingP.attributes",
	},
	request{
		name:   "mcp-global",
		class:  "mcpInstPol",
		filter: "#.mcpInstPol.attributes",
	},
	request{
		name:   "mcp-interface",
		class:  "mcpIfPol",
		filter: "#.mcpIfPol.attributes",
	},
	request{
		name:   "port-tracking",
		class:  "infraPortTrackPol",
		filter: "#.infraPortTrackPol.attributes",
	},
	request{
		name:   "rogue-ep-control",
		class:  "epControlP",
		filter: "#.epControlP.attributes",
	},
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
	if err != nil {
		fmt.Println("please report the following error:")
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
	fmt.Printf("Please provide %s to Cisco services for further analysis.\n", resultZip)
}
