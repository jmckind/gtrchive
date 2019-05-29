// Copyright 2019 gtrchive authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// 	http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"runtime"

	"github.com/ChimeraCoder/anaconda"
	r "gopkg.in/rethinkdb/rethinkdb-go.v5"
)

var log = anaconda.BasicLogger

func main() {
	log.Debugf("Go Version: %s", runtime.Version())
	log.Debugf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)

	api := anaconda.NewTwitterApiWithCredentials(
		os.Getenv("GTR_TWITTER_ACCESS_TOKEN"),
		os.Getenv("GTR_TWITTER_ACCESS_SECRET"),
		os.Getenv("GTR_TWITTER_CONSUMER_KEY"),
		os.Getenv("GTR_TWITTER_CONSUMER_SECRET"),
	)
	api.Log = log

	if ok, err := api.VerifyCredentials(); !ok || err != nil {
		log.Fatalf("Invalid credentials. %v", err)
	}

	params := url.Values{
		"track": {os.Getenv("GTR_TWITTER_TRACK")},
	}

	rdbHost := os.Getenv("GTR_RETHINKDB_HOST")
	rdbPort := os.Getenv("GTR_RETHINKDB_PORT")
	rdbDatabase := os.Getenv("GTR_RETHINKDB_DATABASE")
	rdbUsername := os.Getenv("GTR_RETHINKDB_USERNAME")
	rdbPassword := os.Getenv("GTR_RETHINKDB_PASSWORD")
	rdbTLSCAPath := os.Getenv("GTR_RETHINKDB_TLS_CA")
	rdbTLSCertPath := os.Getenv("GTR_RETHINKDB_TLS_CERT")
	rdbTLSKeyPath := os.Getenv("GTR_RETHINKDB_TLS_KEY")

	log.Debugf("RethinkDB Host: %s", rdbHost)
	log.Debugf("RethinkDB Port: %s", rdbPort)
	log.Debugf("RethinkDB Database: %s", rdbDatabase)
	log.Debugf("RethinkDB Username: %s", rdbUsername)
	log.Debugf("RethinkDB Password: %s", rdbPassword)
	log.Debugf("RethinkDB TLS CA Path: %s", rdbTLSCAPath)
	log.Debugf("RethinkDB TLS Cert Path: %s", rdbTLSCertPath)
	log.Debugf("RethinkDB TLS Key Path: %s", rdbTLSKeyPath)

	var tlsConfig *tls.Config

	if len(rdbTLSCAPath) > 0 && len(rdbTLSCertPath) > 0 {
		certPool := x509.NewCertPool()
		caCert, err := ioutil.ReadFile(rdbTLSCAPath)
		if err != nil {
			log.Fatalf("Unable to parse CA certificate. %v", err)
		}
		certPool.AppendCertsFromPEM(caCert)

		clientCert, err := tls.LoadX509KeyPair(rdbTLSCertPath, rdbTLSKeyPath)
		if err != nil {
			log.Fatalf("Unable to parse client key pair. %v", err)
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      certPool,
		}
	}

	rdbOpts := r.ConnectOpts{
		Address:   fmt.Sprintf("%s:%s", rdbHost, rdbPort),
		Database:  rdbDatabase,
		Username:  rdbUsername,
		Password:  rdbPassword,
		TLSConfig: tlsConfig,
	}

	session, err := r.Connect(rdbOpts)
	if err != nil {
		log.Fatalf("Unable to connect to database. %v", err)
	}

	err = r.DBCreate(rdbDatabase).Exec(session)
	if err != nil {
		log.Errorf("Unable to create database. %v", err)
	}

	err = r.TableCreate("tweets").Exec(session)
	if err != nil {
		log.Errorf("Unable to create table. %v", err)
	}

	log.Debugf("Streaming tweets using params: %v", params)
	stream := api.PublicStreamFilter(params)

	for obj := range stream.C {
		switch o := obj.(type) {
		case anaconda.Tweet:
			log.Debugf("%-15s: %s", o.User.ScreenName, o.Text)
			err := r.Table("tweets").Insert(o).Exec(session)
			if err != nil {
				log.Errorf("Unable to insert database record. %v", err)
			}
		}
	}
}
