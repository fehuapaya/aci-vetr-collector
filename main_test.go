package main

import (
	"testing"
	"time"

	"github.com/brightpuddle/goaci"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/buntdb"
	"github.com/tidwall/gjson"
	"gopkg.in/h2non/gock.v1"
)

func TestFetch(t *testing.T) {
	a := assert.New(t)
	defer gock.Off()
	gock.New("https://apic").
		Get("/api/class/fvTenant.json").
		Reply(200).
		BodyString(goaci.Body{}.
			Set("imdata.0.fvTenant.attributes.dn", "uni/tn-zero").
			Set("imdata.1.fvTenant.attributes.dn", "uni/tn-one").
			Str)

	client, _ := goaci.NewClient("apic", "usr", "pwd")
	client.LastRefresh = time.Now()
	gock.InterceptClient(client.HttpClient)
	db, _ := buntdb.Open(":memory:")
	req := newRequest("fvTenant")
	wg.Add(1)
	fetch(client, req, db)
	err := db.View(func(tx *buntdb.Tx) error {
		return tx.AscendKeys("fvTenant:*", func(key, value string) bool {
			a.Equal(key, "fvTenant:"+gjson.Get(value, "dn").Str)
			return true
		})
	})
	a.NoError(err)

	// name     string             // Custom class name for DB - use class by default
	// class    string             // MO class to query
	// queries  []func(*goaci.Req) // query paramters
	// filter   string             // GJSON path query for the result
	// optional bool               // Fail on unsuccessful collection?
}
