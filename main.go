// Copyright (c) 2014 The SkyDNS Authors. All rights reserved.
// Use of this source code is governed by The MIT License (MIT) that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
)

var (
	machines    = strings.Split(os.Getenv("ETCD_MACHINES"), ",")      // list of URLs to etcd
	nameservers = strings.Split(os.Getenv("SKYDNS_NAMESERVERS"), ",") // list of nameservers
	tlskey      = os.Getenv("ETCD_TLSKEY")                            // TLS private key path
	tlspem      = os.Getenv("ETCD_TLSPEM")                            // X509 certificate
	config      = &Config{ReadTimeout: 0, Domain: "", DnsAddr: "", DNSSEC: ""}
	nameserver  = ""
	machine     = ""
	discover    = false
	verbose     = false
)

const (
	SCacheCapacity = 10000
	RCacheCapacity = 100000
	RCacheTtl      = 60
)

func init() {
	flag.StringVar(&config.Domain, "domain",
		func() string {
			if x := os.Getenv("SKYDNS_DOMAIN"); x != "" {
				return x
			}
			return "skydns.local."
		}(), "domain to anchor requests to (SKYDNS_DOMAIN)")
	flag.StringVar(&config.DnsAddr, "addr",
		func() string {
			if x := os.Getenv("SKYDNS_ADDR"); x != "" {
				return x
			}
			return "127.0.0.1:53"
		}(), "ip:port to bind to (SKYDNS_ADDR)")

	flag.StringVar(&nameserver, "nameservers", "", "nameserver address(es) to forward (non-local) queries to e.g. 8.8.8.8:53,8.8.4.4:53")
	flag.StringVar(&machine, "machines", "", "machine address(es) running etcd")
	flag.StringVar(&config.DNSSEC, "dnssec", "", "basename of DNSSEC key file e.q. Kskydns.local.+005+38250")
	flag.StringVar(&tlskey, "tls-key", "", "TLS Private Key path")
	flag.StringVar(&tlspem, "tls-pem", "", "X509 Certificate")
	flag.DurationVar(&config.ReadTimeout, "rtimeout", 2*time.Second, "read timeout")
	flag.BoolVar(&config.RoundRobin, "round-robin", true, "round robin A/AAAA replies")
	flag.BoolVar(&discover, "discover", false, "discover new machines by watching /v2/_etcd/machines")
	flag.BoolVar(&verbose, "verbose", false, "log queries")

	// TTl
	// Minttl
	flag.StringVar(&config.Hostmaster, "hostmaster", "hostmaster@skydns.local.", "hostmaster email address to use")
	flag.IntVar(&config.SCache, "scache", SCacheCapacity, "capacity of the signature cache")
	flag.IntVar(&config.RCache, "rcache", 0, "capacity of the response cache") // default to 0 for now
	flag.IntVar(&config.RCacheTtl, "rcache-ttl", RCacheTtl, "TTL of the response cache")
}

func main() {
	flag.Parse()
	client := NewClient(machines)
	if nameserver != "" {
		config.Nameservers = strings.Split(nameserver, ",")
	}
	config, err := loadConfig(client, config)
	if err != nil {
		log.Fatal(err)
	}
	s := NewServer(config, client)

	if discover {
		go func() {
			recv := make(chan *etcd.Response)
			go s.client.Watch("/_etcd/machines/", 0, true, recv, nil)
			for {
				select {
				case n := <-recv:
					// we can see an n == nil, probably when we can't connect to etcd.
					if n != nil {
						s.UpdateClient(n)
					}
				}
			}
		}()
	}

	statsCollect()

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
