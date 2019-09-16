package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"golang.org/x/net/dns/dnsmessage"
)

const (
	TTL = 5
)

type request struct {
	addr *net.UDPAddr
	id   uint16
}

type DNSServer struct {
	upstream      net.IP
	conn          *net.UDPConn
	proxyResponse *dnsmessage.AResource
	toFilter      []string

	mtx       sync.Mutex
	idCounter uint16
	requests  map[uint16]request
}

func NewDNS(upstreamIP, proxyIP string, toFilter []string) (*DNSServer, error) {
	upstream := net.ParseIP(upstreamIP)
	if upstream == nil {
		return nil, fmt.Errorf("invalid upstream dns server: %q", upstreamIP)
	}

	proxy := net.ParseIP(proxyIP)
	if proxy == nil {
		return nil, fmt.Errorf("invalid proxy server: %q", proxyIP)
	}
	proxy = proxy.To4()
	if proxy == nil {
		return nil, fmt.Errorf("invalid proxy server: %q", proxyIP)
	}

	var arecord dnsmessage.AResource
	if copy(arecord.A[:], proxy) != 4 {
		return nil, fmt.Errorf("failed writing proxy body")
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 53})
	if err != nil {
		return nil, err
	}
	return &DNSServer{
		upstream:      upstream,
		conn:          conn,
		proxyResponse: &arecord,
		toFilter:      toFilter,
		requests:      map[uint16]request{}}, nil
}

func (s *DNSServer) Addr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *DNSServer) Close() error {
	return s.conn.Close()
}

func (s *DNSServer) Run(ctx context.Context) error {
	var buf [4096]byte
	for {
		n, addr, err := s.conn.ReadFromUDP(buf[:])
		if err != nil {
			// TODO: temporary errors
			return err
		}
		var msg dnsmessage.Message
		err = msg.Unpack(buf[:n])
		if err != nil {
			fmt.Println("invalid dns packet: %v", err)
			continue
		}

		go func() {
			err := s.route(ctx, addr, &msg)
			if err != nil {
				fmt.Println("error: %v", err)
			}
		}()
	}

	return nil
}

func (s *DNSServer) route(ctx context.Context, source *net.UDPAddr,
	msg *dnsmessage.Message) error {
	if msg.Response {
		return s.proxy(ctx, source, msg)
	}
	return s.query(ctx, source, msg)
}

func (s *DNSServer) query(ctx context.Context, source *net.UDPAddr,
	msg *dnsmessage.Message) error {

	{
		s.mtx.Lock()

		r := request{
			addr: source,
			id:   msg.ID,
		}

		for {
			if _, exists := s.requests[s.idCounter]; !exists {
				break
			}
			s.idCounter += 1
		}
		msg.ID = s.idCounter
		s.idCounter += 1

		s.requests[msg.ID] = r

		s.mtx.Unlock()
	}

	packed, err := msg.Pack()
	if err != nil {
		return err
	}

	_, err = s.conn.WriteToUDP(packed, &net.UDPAddr{IP: s.upstream, Port: 53})
	return err
}

func (s *DNSServer) proxy(ctx context.Context, source *net.UDPAddr,
	msg *dnsmessage.Message) error {

	s.mtx.Lock()
	r, exists := s.requests[msg.ID]
	if !exists {
		s.mtx.Unlock()
		return nil
	}

	delete(s.requests, msg.ID)
	s.mtx.Unlock()

	msg.ID = r.id

	for i, ans := range msg.Answers {
		if ans.Header.Class != dnsmessage.ClassINET {
			continue
		}
		switch ans.Header.Type {
		default:
			continue
		case dnsmessage.TypeA, dnsmessage.TypeAAAA, dnsmessage.TypeCNAME:
		}
		name := ans.Header.Name.String()
		for _, suffix := range s.toFilter {
			if strings.HasSuffix(name, suffix) {
				msg.Answers[i].Header.TTL = TTL
				msg.Answers[i].Body = s.proxyResponse
				break
			}
		}
	}

	packed, err := msg.Pack()
	if err != nil {
		return err
	}

	_, err = s.conn.WriteToUDP(packed, r.addr)
	return err
}
