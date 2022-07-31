package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"

	mybot "lombot/myBot"

	_ "github.com/mattn/go-sqlite3"
)

func OpenSqlite() (*sql.DB, error) {
	log.Println("=> open db connection")

	f, err := os.OpenFile("foo.sqlite", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	f.Close()

	dbConnString := "file:foo.sqlite?cache=shared&mode=rwc"

	db, err := sql.Open("sqlite3", dbConnString)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(4)

	_, err = db.Exec("DROP TABLE IF EXISTS `pse`; CREATE TABLE IF NOT EXISTS `subscriptions` ( `room_id` INTEGER, `user_name` TEXT, PRIMARY KEY (`room_id`, `user_name`) ); CREATE TABLE `pse` ( `id` INTEGER PRIMARY KEY, `name` TEXT, `website` TEXT, `company` TEXT, `location` TEXT CHECK( location IN ('Domestik','Asing') ) ); CREATE INDEX IF NOT EXISTS `pse_index_0` ON `pse` (`name`); CREATE INDEX IF NOT EXISTS `pse_index_1` ON `pse` (`website`); CREATE INDEX IF NOT EXISTS `pse_index_2` ON `pse` (`company`); ")
	if err != nil {
		return nil, err
	}
	return db, nil
}

var client = http.DefaultClient

type helperPSE struct {
	url      string
	location string
	index    int
}

func UpdatePseSqlite(mb *mybot.MyBot, pses []PSETerdaftar) error {
	log.Println("loading data pse")
	mb.Mutex.Lock()
	defer mb.Mutex.Unlock()
	_, err := mb.Db.Exec("DELETE FROM pse; VACUUM;")
	if err != nil {
		return err
	}

	max := 10000
	worker := runtime.NumCPU()
	ch := make(chan helperPSE, worker)
	var wg sync.WaitGroup

	for i := 0; i < worker; i++ {
		go Scrape(mb.Db, ch, &wg)
	}

	for _, pse := range pses {
		max = 10000
		for i := 0; i < max; i++ {
			if i == 0 {
				url := fmt.Sprintf("%s%d.json", pse.Url, i)
				resp, err := client.Get(url)
				if err != nil {
					return err
				}

				var pseResponse PseResponse
				err = json.NewDecoder(resp.Body).Decode(&pseResponse)
				resp.Body.Close()
				if err != nil {
					return err
				}

				max = pseResponse.Meta.Page.LastPage
			}
			wg.Add(1)
			ch <- helperPSE{pse.Url, pse.Location, i}
		}
	}
	wg.Wait()
	close(ch)
	return nil
}

func Scrape(db *sql.DB, ch <-chan helperPSE, wg *sync.WaitGroup) {
	for pse := range ch {
		url := fmt.Sprintf("%s%d.json", pse.url, pse.index)
		resp, err := client.Get(url)
		if err != nil {
			log.Println(err)
			wg.Done()
			continue
		}

		var pseResponse PseResponse
		err = json.NewDecoder(resp.Body).Decode(&pseResponse)
		resp.Body.Close()
		if err != nil {
			log.Println(err)
			wg.Done()
			continue
		}

		count := 0
		query := "INSERT INTO pse (name, company, location, website) VALUES "
		values := make([]string, 0, 51)
		args := make([]any, 0, 51*3)
		for _, data := range pseResponse.Data {
			args = append(args, data.Attributes.Nama, data.Attributes.NamaPerusahaan, pse.location, data.Attributes.Website)
			values = append(values, "(?, ?, ?, ?)")
			count++

			if count >= 50 {
				queryValues := strings.Join(values, ",")
				_, err := db.Exec(query+queryValues, args...)
				if err != nil {
					log.Println(err)
					continue
				}
				values = make([]string, 0, 51)
				args = make([]any, 0, 51*3)
				count = 0
			}
		}

		if len(values) > 0 {
			queryValues := strings.Join(values, ",")
			_, err = db.Exec(query+queryValues, args...)
			if err != nil {
				log.Println(err)
			}
		}
		wg.Done()
	}
}
