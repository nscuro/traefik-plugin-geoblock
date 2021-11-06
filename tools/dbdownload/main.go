package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/ip2location/ip2location-go/v9"
)

func main() {
	var outFilePath string

	flag.StringVar(&outFilePath, "o", "", "Output file path")
	flag.Parse()

	if outFilePath == "" {
		log.Fatalln("no output file path provided")
	}

	log.Println("downloading database archive")
	res, err := http.Get("https://download.ip2location.com/lite/IP2LOCATION-LITE-DB1.IPV6.BIN.ZIP")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, res.Body)
	if err != nil {
		log.Fatal(err)
	}

	reader := bytes.NewReader(buf.Bytes())
	zipReader, err := zip.NewReader(reader, int64(reader.Len()))
	if err != nil {
		log.Fatalf("opening response as zip archive failed: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "ip2location_*")
	if err != nil {
		log.Fatalf("creating temporary file failed: %v", err)
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	found := false
	for _, file := range zipReader.File {
		if file.Name == "IP2LOCATION-LITE-DB1.IPV6.BIN" {
			log.Println("extracting database file")

			fileReader, err := file.Open()
			if err != nil {
				log.Fatal(err)
			}
			defer fileReader.Close()

			_, err = io.Copy(tmpFile, fileReader) // #nosec: G110
			if err != nil {
				log.Fatal(err)
			}

			found = true
			break
		}
	}
	if !found {
		log.Fatal("db file not found in downloaded archive")
	}

	log.Println("verifying database")
	err = verifyDatabase(tmpFile.Name())
	if err != nil {
		log.Fatalf("database is invalid: %v", err)
	}

	// Reset file pointer of tmpFile to the beginning,
	// so we can read from it
	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("writing output file")
	outFile, err := os.Create(outFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, tmpFile)
	if err != nil {
		log.Fatal(err)
	}
}

func verifyDatabase(filePath string) error {
	db, err := ip2location.OpenDB(filePath)
	if err != nil {
		return fmt.Errorf("opening db failed: %w", err)
	}
	defer db.Close()

	rec, err := db.Get_country_short("1.1.1.1")
	if err != nil {
		return fmt.Errorf("querying db failed: %w", err)
	}

	if rec.Country_short != "US" {
		return fmt.Errorf("query returned unexpected result, db is likely corrupted")
	}

	return nil
}
