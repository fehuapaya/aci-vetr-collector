package main

// func TestFetch(t *testing.T) {
// 	a := assert.New(t)
// 	defer gock.Off()
// 	gock.New("https://apic").
// 		Get("/api/class/fvTenant.json").
// 		Reply(200).
// 		BodyString(goaci.Body{}.
// 			Set("imdata.0.fvTenant.attributes.dn", "uni/tn-zero").
// 			Set("imdata.1.fvTenant.attributes.dn", "uni/tn-one").
// 			Str)

// 	client, _ := goaci.NewClient("apic", "usr", "pwd")
// 	client.LastRefresh = time.Now()
// 	gock.InterceptClient(client.HttpClient)
// 	db, _ := buntdb.Open(":memory:")
// 	req = []Request{newRequest("fvTenant")}
// 	fetchHttp(lient{client: client}, req, db)
// 	err := db.View(func(tx *buntdb.Tx) error {
// 		return tx.AscendKeys("fvTenant:*", func(key, value string) bool {
// 			a.Equal(key, "fvTenant:"+gjson.Get(value, "dn").Str)
// 			return true
// 		})
// 	})
// 	a.NoError(err)
// }
