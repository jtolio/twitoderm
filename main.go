package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"
)

var (
	flagAddrs = flag.String("addrs", ":80,:443",
		"comma-separated list of addresses to listen on")
	flagUpstreamDNS = flag.String("upstream-dns", "8.8.8.8",
		"address of upstream resolver")
	flagProxyIP = flag.String("proxy-ip", "127.0.0.1", "address of proxier")
	flagSpeed   = flag.Int("speed", 1024,
		"bytes per sec to allow in a single direction")
	flagConnDelay = flag.Duration("conn_delay", 10*time.Second, "connection delay")
	flagFilter    = flag.String("filter", "twitter.com,twimg.com",
		"comma-separated list of domains to filter")
	flagDNSPort = flag.Int("dns-port", 53, "dns port to listen on")
)

func main() {
	ctx := context.Background()
	flag.Parse()

	domains := strings.Split(*flagFilter, ",")
	for i, domain := range domains {
		if !strings.HasSuffix(domain, ".") {
			domains[i] += "."
		}
	}

	dns, err := NewDNS(*flagUpstreamDNS, *flagProxyIP, *flagDNSPort, domains)
	if err != nil {
		panic(err)
	}
	defer dns.Close()
	fmt.Printf("listening on %q\n", dns.Addr().String())

	addrs := strings.Split(*flagAddrs, ",")
	errs := make(chan error, len(addrs)+1)
	for _, addr := range addrs {
		go func(addr string) {
			errs <- NewProxier(addr, *flagProxyIP, *flagSpeed, *flagConnDelay).Run(ctx)
		}(addr)
	}
	go func() {
		errs <- dns.Run(ctx)
	}()
	panic(<-errs)
}
