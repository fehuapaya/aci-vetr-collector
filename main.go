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

const schemaVersion = 3
const version = "0.1.4"
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
	request{name: "epgs", class: "fvEpP"},
	request{name: "bds", class: "fvBD"},
	request{name: "vrfs", class: "fvCtx"},
	request{name: "l3outs", class: "l3extOut"},
	request{name: "l3-node-profiles", class: "l3extLNodeP"},
	request{name: "l3-int-profiles", class: "l3extLIfP"},
	request{name: "ext-epgs", class: "l3extInstP"},
	request{name: "tenants", class: "fvTenant"},
	// TODO contracts....
	// fzBrCp (contract)
	// fzSubj (subject)
	// fzSubjFiltAtt (filter)

	// Infrastructure
	request{name: "devices", class: "topSystem"},
	request{name: "switch-hardware", class: "fabricNode"},
	request{name: "apic-hardware", class: "eqptBoard"},
	request{name: "pods", class: "fabricSetupP"},

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
	request{
		name:  "stats-by-node",
		class: "eqptcapacityEntity",
		query: []string{
			"query-target=self",
			"rsp-subtree-include=stats",
			fmt.Sprintf("rsp-subtree-class=%s", strings.Join([]string{
				"eqptcapacityL2RemoteUsage5min",
				"eqptcapacityL2TotalUsage5min",
				"eqptcapacityL2Usage5min",
				"eqptcapacityL3RemoteUsage5min",
				"eqptcapacityL3RemoteUsageCap5min",
				"eqptcapacityL3TotalUsage5min",
				"eqptcapacityL3TotalUsageCap5min",
				"eqptcapacityL3Usage5min",
				"eqptcapacityL3UsageCap5min",
				"eqptcapacityMcastUsage5min",
				"eqptcapacityPolUsage5min",
				"eqptcapacityVlanUsage5min",
			}, ",")),
		},
	},

	// Global config
	request{name: "crypto-key", class: "pkiExportEncryptionKey"},
	request{name: "fabric-wide-settings", class: "infraSetPol"},
	request{name: "ep-loop-control", class: "epLoopProtectP"},
	request{name: "rogue-ep-control", class: "epControlP"},
	request{name: "ip-aging", class: "epIpAgingP"},
	request{name: "port-tracking", class: "infraPortTrackPol"},
	request{name: "bgp-route-reflectors", class: "bgpRRNodePEp"},
}

func writeFile(name string, body []byte) string {
	fn := fmt.Sprintf("%s.json", name)
	if err := ioutil.WriteFile(fn, body, 0644); err != nil {
		log.Panic(err)
	}
	return fn
}

func fetch(client aci.Client, req aci.Req) aci.Res {
	// TODO provide an async client.Get interface
	res, err := client.Get(req)
	if err != nil {
		fmt.Println("Please report the following error.")
		fmt.Printf("%+v\n", req)
		log.Fatal(err)
	}
	return res
}

func zipFiles(files []string) {
	if err := archiver.Archive(files, resultZip); err != nil {
		log.Panic(err)
	}
}

func rmTempFiles(files []string) {
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			log.Panic(err)
		}
	}
}

func main() {
	var files []string
	cfg := newConfigFromCLI()
	client := aci.NewClient(aci.Config{
		IP:             cfg.IP,
		Username:       cfg.Username,
		Password:       cfg.Password,
		RequestTimeout: 90,
	})
	fmt.Printf("Authenticating to the APIC...")
	if err := client.Login(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("[ok]")
	fmt.Printf("Querying fabric...")
	for _, req := range reqs {
		fmt.Printf(".")
		// TODO support class queries directly in the go-aci lib
		res := fetch(client, aci.Req{
			URI:   fmt.Sprintf("/api/class/%s", req.class),
			Query: req.query,
		})
		data := res.Raw
		if cfg.Pretty {
			data = res.Get("@pretty").Raw
		}
		files = append(files, writeFile(req.name, []byte(data)))
	}
	fmt.Println("[ok]")
	metadata, _ := json.Marshal(map[string]interface{}{
		"collectorVersion": version,
		"schemaVersion":    schemaVersion,
		"timestamp":        time.Now(),
	})
	files = append(files, writeFile("meta", metadata))
	fmt.Printf("Creating archive...")
	zipFiles(files)
	fmt.Println("[ok]")
	fmt.Printf("Removing temp files...")
	rmTempFiles(files)
	fmt.Println("[ok]")
	fmt.Println("Collection complete.")
	fmt.Printf("Please provide %s to Cisco services for further analysis.\n", resultZip)
}
