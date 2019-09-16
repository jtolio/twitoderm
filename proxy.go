package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/juju/ratelimit"
	"github.com/spacemonkeygo/tlshowdy"
)

type Proxier struct {
	addr        string
	bytesPerSec int
	connDelay   time.Duration

	mtx     sync.Mutex
	buckets map[string]*ratelimit.Bucket
}

func NewProxier(addr string, bytesPerSec int, connDelay time.Duration) *Proxier {
	return &Proxier{
		addr:        addr,
		bytesPerSec: bytesPerSec,
		connDelay:   connDelay,
		buckets:     map[string]*ratelimit.Bucket{}}
}

func (p *Proxier) slowDown(host string, r io.Reader) io.Reader {
	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	host = strings.Join(parts, ".")

	p.mtx.Lock()
	defer p.mtx.Unlock()

	bucket, exists := p.buckets[host]
	if !exists {
		bucket = ratelimit.NewBucketWithRate(float64(p.bytesPerSec), int64(p.bytesPerSec))
		p.buckets[host] = bucket
	}

	return ratelimit.Reader(r, bucket)
}

func (p *Proxier) proxyConn(ctx context.Context, host string, conn net.Conn) error {
	fmt.Println("+", host)
	defer fmt.Println("-", host)

	time.Sleep(p.connDelay)

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
		_, err := io.Copy(remote, p.slowDown(host, conn))
		errs <- err
	}()

	go func() {
		_, err := io.Copy(conn, p.slowDown(host, remote))
		errs <- err
	}()

	<-errs
	return nil
}

func (p *Proxier) handleConn(ctx context.Context, conn net.Conn) error {
	recorder := tlshowdy.NewRecordingReader(conn)
	hello, err := tlshowdy.Read(recorder)
	if err != nil {
		return err
	}
	if hello != nil {
		return p.proxyConn(ctx, hello.ServerName, tlshowdy.NewPrefixConn(recorder.Received, conn))
	}
	host, err := httpHost(io.MultiReader(bytes.NewReader(append([]byte(nil), recorder.Received...)), recorder))
	if err != nil {
		return err
	}
	return p.proxyConn(ctx, host, tlshowdy.NewPrefixConn(recorder.Received, conn))
}

func (p *Proxier) Run(ctx context.Context) error {
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", p.addr)
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
			err := p.handleConn(ctx, conn)
			if err != nil {
				fmt.Printf("failed handling conn: %v\n", err)
			}
		}()
	}

	return nil
}
