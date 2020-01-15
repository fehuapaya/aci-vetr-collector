package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/brightpuddle/goaci"
	"github.com/mholt/archiver"
	"github.com/rs/zerolog"
	"github.com/tidwall/buntdb"
	"golang.org/x/sync/errgroup"
)

// Version comes from CI
var version string

const (
	resultZip = "aci-vetr-data.zip"
	logFile   = "aci-vetr-c.log"
	dbName    = "data.db"
)

// Write requests to icurl script to be run on the APIC.
// Note, this is a more complicated collection methodology and should rarely
// be used.
func writeICurl(args Args, log zerolog.Logger) error {
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

// Write results to db file.
func writeToDB(results map[string]goaci.Res) error {
	db, err := buntdb.Open(dbName)
	if err != nil {
		return fmt.Errorf("cannot open output file: %v", err)
	}
	defer db.Close()

	for prefix, res := range results {
		if err := db.Update(func(tx *buntdb.Tx) error {
			for _, record := range res.Get("imdata.#.*.attributes").Array() {
				dn := record.Get("dn").Str
				if dn == "" {
					return fmt.Errorf("DN empty: %s", record.Raw)
				}
				key := fmt.Sprintf("%s:%s", prefix, record.Get("dn").Str)
				if _, _, err := tx.Set(key, record.Raw, nil); err != nil {
					return fmt.Errorf("cannot set key: %v", err)
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("cannot write to DB file: %v", err)
		}
	}

	// Add metadata
	metadata := goaci.Body{}.
		Set("collectorVersion", version).
		Set("timestamp", time.Now().String()).
		Str
	if err := db.Update(func(tx *buntdb.Tx) error {
		if _, _, err := tx.Set("meta", string(metadata), nil); err != nil {
			return fmt.Errorf("cannot write metadata to db: %v", err)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// Fetch data via API.
func fetchHttp(args Args, log zerolog.Logger) error {
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

	// Fetch data from API
	fmt.Println(strings.Repeat("=", 30))

	results := make(map[string]goaci.Res)
	var g errgroup.Group

	for _, req := range reqs {
		req := req

		g.Go(func() error {
			startTime := time.Now()
			log.Debug().Time("start_time", startTime).Msgf("begin: %s", req.prefix)

			log.Info().Str("class", req.prefix).Msg("fetching resource...")
			log.Debug().Str("url", req.req.HttpReq.URL.String()).Msg("requesting resource")

			res, err := client.Do(req.req)
			if err != nil {
				return fmt.Errorf("failed to make request: %v", err)
			}
			results[req.prefix] = res
			log.Debug().
				TimeDiff("elapsed_time", time.Now(), startTime).
				Msgf("done: %s", req.prefix)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if err := writeToDB(results); err != nil {
		return fmt.Errorf("error writing to DB: %v", err)
	}

	fmt.Println(strings.Repeat("=", 30))

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
	log := NewLogger()
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
