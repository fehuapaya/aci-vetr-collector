package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/brightpuddle/goaci"
	"github.com/mholt/archiver"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/buntdb"
)

// Version comes from CI
var version string

const (
	resultZip = "aci-vetr-data.zip"
	logFile   = "aci-vetr-c.log"
	dbName    = "data.db"
)

var wg sync.WaitGroup

type Client struct {
	client goaci.Client
	log    Logger
}

func (client Client) request(req Request) (goaci.Res, error) {
	startTime := time.Now()
	client.log.Debug().Time("start_time", startTime).Msgf("begin: %s", req.prefix)
	res, err := client.client.Do(req.req)
	client.log.Debug().
		TimeDiff("elapsed_time", time.Now(), startTime).
		Msgf("done: %s", req.prefix)
	return res, err
}

func fetch(client Client, req Request, db *buntdb.DB) {
	client.log.Info().Str("class", req.prefix).Msg("fetching resource...")
	client.log.Debug().
		Str("url", req.req.HttpReq.URL.String()).
		Msg("requesting resource")
	res, err := client.request(req)
	if err != nil {
		client.log.Error().
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

// Write requests to icurl script to be run on the APIC.
// Note, this is a more complicated collection methodology and should rarely
// be used.
func writeICurl(args Args, log Logger) error {
	var (
		fn        = "vetr-collect.sh"
		final     = "aci-vetr-raw.zip"
		tmpFolder = "/tmp/aci-vetr-collections"
	)
	os.Remove(fn)
	script := []string{
		"#!/bin/bash",
		"",
		"mkdir " + tmpFolder,
		"",
		"# Fetch data from API",
	}

	for _, req := range reqs {
		cmd := fmt.Sprintf("icurl -kG https://localhost/%s", req.req.HttpReq.URL.Path)

		for key, value := range req.req.HttpReq.URL.Query() {
			if len(value) >= 1 {
				cmd = fmt.Sprintf("%s -d '%s=%s'", cmd, key, value[0])
			}
		}
		outFile := req.prefix + ".json"
		cmd = fmt.Sprintf("%s > %s/%s", cmd, tmpFolder, outFile)
		script = append(script, cmd)
	}

	script = append(script, []string{
		"",
		"# Zip result",
		fmt.Sprintf("zip -mj ~/%s %s/*.json", final, tmpFolder),
		"",
		"# Cleanup",
		"rm -rf " + tmpFolder,
		"",
		"echo Collection complete.",
		fmt.Sprintf("echo Provide Cisco Services the %s file.", final),
	}...)

	err := ioutil.WriteFile(fn, []byte(strings.Join(script, "\n")), 0755)
	if err != nil {
		return err
	}
	log.Info().Msgf("Script complete. Run %s on the APIC.", fn)
	return nil
}

// Fetch data via API.
func fetchHttp(args Args, log Logger) error {
	client, err := goaci.NewClient(
		args.APIC,
		args.Username,
		args.Password,
		goaci.RequestTimeout(600),
	)
	if err != nil {
		return fmt.Errorf("failed to create ACI client: %v", err)
	}

	// Authenticate
	log.Info().Str("host", args.APIC).Msg("APIC host")
	log.Info().Str("user", args.Username).Msg("APIC username")
	log.Info().Msg("Authenticating to the APIC...")
	if err := client.Login(); err != nil {
		return fmt.Errorf("cannot authenticate to the APIC at %s: %v", args.APIC, err)
	}

	db, err := buntdb.Open(dbName)
	if err != nil {
		return fmt.Errorf("cannot open output file: %v", err)
	}

	// Fetch data from API
	fmt.Println(strings.Repeat("=", 30))
	for _, req := range reqs {
		wg.Add(1)
		go fetch(Client{client: client, log: log}, req, db)
	}
	wg.Wait()

	fmt.Println(strings.Repeat("=", 30))

	// Add metadata
	metadata := goaci.Body{}.
		Set("collectorVersion", version).
		Set("timestamp", time.Now().String()).
		Str
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
		return fmt.Errorf("cannot create archive: %v", err)
	}

	// Cleanup
	fmt.Println(strings.Repeat("=", 30))
	log.Info().Msg("Collection complete.")
	log.Info().Msgf("Please provide %s to Cisco Services for further analysis.", args.Output)
	return nil
}

func main() {
	log := newLogger()
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				log.Error().Err(err).Msg("unexpected error")
			}
			log.Error().Msg("Collection failed.")
		}
		fmt.Println("Press enter to exit.")
		var throwaway string
		fmt.Scanln(&throwaway)
	}()
	args, err := newArgs()
	if err != nil {
		panic(err)
	}
	if args.ICurl {
		err := writeICurl(args, log)
		if err != nil {
			log.Error().Err(err).Msg("cannot create icurl script")
		}
	} else {
		err := fetchHttp(args, log)
		if err != nil {
			log.Error().Err(err).Msg("cannot fetch data from the API")
		}
	}
}
