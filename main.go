package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"

	"github.com/juju/ratelimit"
	"github.com/spacemonkeygo/tlshowdy"
)

var (
	flagAddrs = flag.String("addrs", ":80,:443",
		"comma-separated list of addresses to listen on")
	flagSpeed = flag.Int("speed", 1024, "bytes per sec to allow in a single direction")

	hostRegex = regexp.MustCompile(`^Host:\s+(.*?)$`)
)

func SlowDown(r io.Reader) io.Reader {
	return ratelimit.Reader(r, ratelimit.NewBucketWithRate(float64(*flagSpeed), int64(*flagSpeed)))
}

func ProxyConn(ctx context.Context, host string, conn net.Conn) error {
	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return err
	}

	remote, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	defer remote.Close()
	defer conn.Close()

	errs := make(chan error, 2)

	go func() {
		_, err := io.Copy(remote, SlowDown(conn))
		errs <- err
	}()

	go func() {
		_, err := io.Copy(conn, SlowDown(remote))
		errs <- err
	}()

	return <-errs
}

func HTTPHost(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			return "", nil
		}
		host := hostRegex.FindStringSubmatch(text)
		if len(host) > 1 {
			return host[1], nil
		}
	}
	return "", scanner.Err()
}

func HandleConn(ctx context.Context, conn net.Conn) error {
	recorder := tlshowdy.NewRecordingReader(conn)
	hello, err := tlshowdy.Read(recorder)
	if err != nil {
		return err
	}
	if hello != nil {
		return ProxyConn(ctx, hello.ServerName, tlshowdy.NewPrefixConn(recorder.Received, conn))
	}
	host, err := HTTPHost(io.MultiReader(bytes.NewReader(append([]byte(nil), recorder.Received...)), recorder))
	if err != nil {
		return err
	}
	return ProxyConn(ctx, host, tlshowdy.NewPrefixConn(recorder.Received, conn))
}

func AcceptLoop(ctx context.Context, addr string) error {
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer l.Close()

	fmt.Printf("listening on %q\n", l.Addr().String())

	for {
		conn, err := l.Accept()
		if err != nil {
			// TODO: backoff on temporary
			return err
		}
		go func() {
			defer conn.Close()
			err := HandleConn(ctx, conn)
			if err != nil {
				fmt.Printf("failed handling conn: %v\n", err)
			}
		}()
	}

	return nil
}

func main() {
	ctx := context.Background()
	flag.Parse()

	addrs := strings.Split(*flagAddrs, ",")
	errs := make(chan error, len(addrs))
	for _, addr := range addrs {
		go func(addr string) {
			errs <- AcceptLoop(ctx, addr)
		}(addr)
	}
	for range addrs {
		err := <-errs
		if err != nil {
			panic(err)
		}
	}
}
