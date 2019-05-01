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

const version = "0.1.0"
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
	uri   string
	query []string
}

var reqs = []request{
	// Tenant objects
	request{name: "epgs", uri: "/api/class/fvEpP"},
	request{name: "bds", uri: "/api/class/fvBD"},
	request{name: "vrfs", uri: "/api/class/fvCtx"},
	request{name: "L3outs", uri: "/api/class/l3extOut"},
	request{name: "nodeProfiles", uri: "/api/class/l3extLNodeP"},
	request{name: "intProfiles", uri: "/api/class/l3extLIfP"},
	request{name: "extEpgs", uri: "/api/class/l3extInstP"},
	request{name: "tenants", uri: "/api/class/fvBD"},
	// TODO contracts....
	// fzBrCp (contract)
	// fzSubj (subject)
	// fzSubjFiltAtt (filter)

	// Infrastructure
	request{name: "devices", uri: "/api/class/topSystem"},
	request{name: "pods", uri: "/api/class/fabricSetupP"},

	// State
	request{name: "faults", uri: "/api/class/faultInfo"},
	request{name: "capacityRules", uri: "/api/class/fvcapRule"},
	request{
		name:  "epCount",
		uri:   "/api/class/fvEpP",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "ipCount",
		uri:   "/api/class/fvIp",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "l4l7ContainerCount",
		uri:   "/api/class/vnsCDev",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "l4l7ServiceGraphCount",
		uri:   "/api/class/vnsGraphInst",
		query: []string{"rsp-subtree-include=count"},
	},
	request{
		name:  "moCountByNode",
		uri:   "/api/class/ctxClassCnt",
		query: []string{"rsp-subtree-class=l2BD,fvEpP,l3Dom"},
	},
	request{
		name: "statsByNode",
		uri:  "/api/class/eqptcapacityEntity",
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
	request{name: "cryptoKey", uri: "/api/class/pkiExportEncryptionKey"},
	request{name: "fabricWideSettings", uri: "/api/class/infraSetPol"},
	request{name: "epLoopControl", uri: "/api/class/epLoopProtectP"},
	request{name: "rogueEpControl", uri: "/api/class/epControlP"},
	request{name: "ipAging", uri: "/api/class/epIpAgingP"},
	request{name: "portTracking", uri: "/api/class/infraPortTrackPol"},
	request{name: "bgpRRs", uri: "/api/class/bgpRRNodePEp"},
}

func writeFile(name string, body []byte) string {
	fn := fmt.Sprintf("%s.json", name)
	if err := ioutil.WriteFile(fn, body, 0644); err != nil {
		log.Panic(err)
	}
	return fn
}

func fetch(client aci.Client, req aci.Req) aci.Res {
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
		res := fetch(client, aci.Req{URI: req.uri, Query: req.query})
		data := res.Raw
		if cfg.Pretty {
			data = res.Get("@pretty").Raw
		}
		files = append(files, writeFile(req.name, []byte(data)))
	}
	fmt.Println("[ok]")
	metadata, _ := json.Marshal(map[string]interface{}{
		"timestamp": time.Now(),
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
