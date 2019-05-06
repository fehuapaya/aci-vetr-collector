package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/brightpuddle/go-aci"
	"github.com/mholt/archiver"
)

const schemaVersion = 6
const version = "0.1.7"
const resultZip = "health-check-data.zip"

// Config : command line parameters
type Config struct {
	IP       string `arg:"-i" help:"APIC IP address"`
	Username string `arg:"-u"`
	Password string `arg:"-p"`
	Pretty   bool   `help:"pretty print JSON"`
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
	name  string
	class string
	query []string
}

var reqs = []request{
	// Tenant objects
	request{name: "bds", class: "fvBD"},
	request{name: "contracts", class: "vzBrCP"},
	request{name: "epgs", class: "fvEpP"},
	request{name: "ext-epgs", class: "l3extInstP"},
	request{name: "filters", class: "vzRsSubjFiltAtt"},
	request{name: "l3-int-profiles", class: "l3extLIfP"},
	request{name: "l3-node-profiles", class: "l3extLNodeP"},
	request{name: "l3outs", class: "l3extOut"},
	request{name: "subjects", class: "vzSubj"},
	request{name: "tenants", class: "fvTenant"},
	request{name: "vrfs", class: "fvCtx"},

	// Infrastructure
	request{name: "apic-hardware", class: "eqptBoard"},
	request{name: "devices", class: "topSystem"},
	request{name: "pods", class: "fabricSetupP"},
	request{name: "switch-hardware", class: "fabricNode"},

	// State
	request{name: "faults", class: "faultInfo"},
	request{name: "capacity-rules", class: "fvcapRule"},
	request{
		name:  "ep-count",
		class: "fvEpP",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "ip-count",
		class: "fvIp",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "l4l7-container-count",
		class: "vnsCDev",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "l4l7-service-graph-count",
		class: "vnsGraphInst",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "mo-count-by-node",
		class: "ctxClassCnt",
		query: []string{"rsp-subtree-class=l2BD,fvEpP,l3Dom"},
	},
	request{name: "capacity-vlan", class: "eqptcapacityVlanUsage5min"},
	request{name: "capacity-tcam", class: "eqptcapacityPolUsage5min"},
	request{name: "capacity-l2-local", class: "eqptcapacityL2Usage5min"},
	request{name: "capacity-l2-remote", class: "eqptcapacityL2RemoteUsage5min"},
	request{name: "capacity-l2-total", class: "eqptcapacityL2TotalUsage5min"},
	request{name: "capacity-l3-local", class: "eqptcapacityL3Usage5min"},
	request{name: "capacity-l3-remote", class: "eqptcapacityL3RemoteUsage5min"},
	request{name: "capacity-l3-total", class: "eqptcapacityL3TotalUsage5min"},
	request{name: "capacity-l3-local-cap", class: "eqptcapacityL3UsageCap5min"},
	request{name: "capacity-l3-remote-cap", class: "eqptcapacityL3RemoteUsageCap5min"},
	request{name: "capacity-l3-total-cap", class: "eqptcapacityL3TotalUsageCap5min"},
	request{name: "capacity-mcast", class: "eqptcapacityMcastUsage5min"},

	// Global config
	request{name: "bgp-route-reflectors", class: "bgpRRNodePEp"},
	request{name: "crypto-key", class: "pkiExportEncryptionKey"},
	request{name: "ep-loop-control", class: "epLoopProtectP"},
	request{name: "fabric-wide-settings", class: "infraSetPol"},
	request{name: "ip-aging", class: "epIpAgingP"},
	request{name: "mcp-global", class: "mcpInstPol"},
	request{name: "mcp-interface", class: "mcpIfPol"},
	request{name: "port-tracking", class: "infraPortTrackPol"},
	request{name: "rogue-ep-control", class: "epControlP"},
}

func writeFile(name string, body []byte) string {
	fn := fmt.Sprintf("%s.json", name)
	if err := ioutil.WriteFile(fn, body, 0644); err != nil {
		log.Panic(err)
	}
	return fn
}

func fetch(client aci.Client, cfg Config, req request, c chan string) {
	fmt.Printf("fetching %s\n", req.name)
	res, err := client.Get(aci.Req{
		URI:   fmt.Sprintf("/api/class/%s", req.class),
		Query: req.query,
	})
	if err != nil {
		fmt.Println("please report the following error:")
		fmt.Printf("%+v\n", req)
		log.Fatal(err)
	}
	data := res.Raw
	if cfg.Pretty {
		data = res.Get("@pretty").Raw
	}
	c <- writeFile(req.name, []byte(data))
}

func zipFiles(files []string) {
	fmt.Println("creating archive")
	os.Remove(resultZip) // Remove any old archives and ignore errors
	if err := archiver.Archive(files, resultZip); err != nil {
		log.Panic(err)
	}
}

func rmTempFiles(files []string) {
	fmt.Println("removing temp files")
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			log.Panic(err)
		}
	}
}

func main() {
	// Setup
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
		log.Fatal(err)
	}

	// Fetch data from API
	fmt.Println(strings.Repeat("=", 30))
	var files []string
	c := make(chan string)
	for _, req := range reqs {
		go fetch(client, cfg, req, c)
	}
	for range reqs {
		files = append(files, <-c)
	}
	fmt.Println(strings.Repeat("=", 30))

	// Append metadata
	metadata, _ := json.Marshal(map[string]interface{}{
		"collectorVersion": version,
		"schemaVersion":    schemaVersion,
		"timestamp":        time.Now(),
	})
	files = append(files, writeFile("meta", metadata))

	// Create archive
	zipFiles(files)
	rmTempFiles(files)
	fmt.Println(strings.Repeat("=", 30))
	fmt.Println("Collection complete.")
	fmt.Printf("Please provide %s to Cisco services for further analysis.\n", resultZip)
}
