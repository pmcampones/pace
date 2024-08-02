package network

import (
	"crypto/ecdsa"
	"crypto/tls"
	"fmt"
	"net"
)

type Peer struct {
	Conn net.Conn
	name string
	pk   *ecdsa.PublicKey
}

func newOutbound(myName, address string, config *tls.Config) (Peer, error) {
	conn, err := tls.Dial("tcp", address, config)
	if err != nil {
		return Peer{}, fmt.Errorf("unable to dial while establishing Peer connection: %v", err)
	}
	err = send(conn, []byte(myName))
	if err != nil {
		return Peer{}, fmt.Errorf("unable to send name of Peer: %v", err)
	}
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Peer{}, fmt.Errorf("no certificates found in connection")
	}
	pk := certs[0].PublicKey.(*ecdsa.PublicKey)
	peer := Peer{
		Conn: conn,
		name: address,
		pk:   pk,
	}
	return peer, nil
}

func getInbound(listener net.Listener) (Peer, error) {
	conn, err := listener.Accept()
	if err != nil {
		return Peer{}, fmt.Errorf("unable to accept inbount connection with Peer: %s", err)
	}
	nameBytes, err := receive(conn)
	if err != nil {
		return Peer{}, fmt.Errorf("unable to receive initialization information of Peer: %s", err)
	}
	name := string(nameBytes)
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Peer{}, fmt.Errorf("no certificates found in connection")
	}
	pk := certs[0].PublicKey.(*ecdsa.PublicKey)
	peer := Peer{
		Conn: conn,
		name: name,
		pk:   pk,
	}
	return peer, nil
}
