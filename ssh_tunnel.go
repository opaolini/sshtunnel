package sshtunnel

import (
	"io"
	"log"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

type SSHTunnel struct {
	Local  *Endpoint
	Server *Endpoint
	Remote *Endpoint
	Config *ssh.ClientConfig
	Log    *log.Logger
	close  chan interface{}
}

func (tunnel *SSHTunnel) logf(fmt string, args ...interface{}) {
	if tunnel.Log != nil {
		tunnel.Log.Printf(fmt, args...)
	}
}

func (tunnel *SSHTunnel) Start() error {
	listener, err := net.Listen("tcp", tunnel.Local.String())
	if err != nil {
		return err
	}
	tunnel.Local.Port = listener.Addr().(*net.TCPAddr).Port
CONNLOOP:
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		tunnel.logf("accepted connection")
		var wg sync.WaitGroup
		go tunnel.forward(conn, &wg)
		wg.Wait()
		tunnel.logf("tunnel closed")
		select {
		case <-tunnel.close:
			break CONNLOOP
		default:
		}
	}
	err = listener.Close()
	if err != nil {
		return err
	}
	return nil
}

func (tunnel *SSHTunnel) forward(localConn net.Conn, wg *sync.WaitGroup) {
	serverConn, err := ssh.Dial("tcp", tunnel.Server.String(), tunnel.Config)
	if err != nil {
		tunnel.logf("server dial error: %s", err)
		return
	}
	tunnel.logf("connected to %s (1 of 2)\n", tunnel.Server.String())
	remoteConn, err := serverConn.Dial("tcp", tunnel.Remote.String())
	if err != nil {
		tunnel.logf("remote dial error: %s", err)
		return
	}
	tunnel.logf("connected to %s (2 of 2)\n", tunnel.Remote.String())
	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			tunnel.logf("io.Copy error: %s", err)
		}
	}
	go copyConn(localConn, remoteConn)
	go copyConn(remoteConn, localConn)
	<-tunnel.close
	tunnel.logf("close signal received, closing...")
	_ = localConn.Close()
	_ = serverConn.Close()
	_ = remoteConn.Close()
	wg.Done()
	return
}

func (tunnel *SSHTunnel) Close() {
	tunnel.close <- struct{}{}
	return
}

func NewSSHTunnel(tunnel string, auth ssh.AuthMethod, destination string) *SSHTunnel {
	// A random port will be chosen for us.
	localEndpoint := NewEndpoint("localhost:0")

	server := NewEndpoint(tunnel)
	if server.Port == 0 {
		server.Port = 22
	}

	sshTunnel := &SSHTunnel{
		Config: &ssh.ClientConfig{
			User: server.User,
			Auth: []ssh.AuthMethod{auth},
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				// Always accept key.
				return nil
			},
		},
		Local:  localEndpoint,
		Server: server,
		Remote: NewEndpoint(destination),
		close:  make(chan interface{}),
	}

	return sshTunnel
}
