package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"code.google.com/p/go.crypto/ssh"
	"code.google.com/p/go.crypto/ssh/terminal"
	"github.com/nitrous-io/nitrous-common/cipher"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
)

var (
	maxAttempts = 3
	wait        = 3 * time.Second
	termMode    = "xterm-256color"
	sshUser     = "root"

	aesKey string
	dbURL  string
)

// represent the ssh connection status. When the connection succeeds, then we
// have the client and err should be nil
type conn struct {
	client *ssh.Client
	err    error
}

type resizeRequest struct {
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

func init() {
	// get AES_Key
	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		log.Fatal("AES_KEY is not defined")
	}

	// get db connection
	dbURL = os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		log.Fatal("NITROUS_PG_URL is not defined")
	}
}

func main() {
	// get host's id
	if len(os.Args) != 2 {
		log.Fatalf("Required host's slug")
	}
	slug := os.Args[1]

	// get db conection
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		log.Fatalf("failed to open db connection", dbConn)
	}
	log.Print("===> established db connection ...")

	// get host
	h, err := host.FindBySlug(dbConn, slug)
	if err != nil || h == nil {
		log.Fatalf("failed to find host with id %d", slug)
	}
	log.Print("===> found host...")

	// get keypair
	kp, err := keypairs.Get(dbConn, h.ID)
	if err != nil {
		log.Fatalf("failed to find a keypair for host %v", h)
	}
	log.Print("===> found keypair...")

	// get the private key from keypair
	priv, err := cipher.Decrypt([]byte(aesKey), []byte(kp.EncryptedPrivateKeyIv), []byte(kp.EncryptedPrivateKey))
	if err != nil {
		log.Fatalf("failed to decrypt private key %v", err)
	}
	log.Print("===> found private key...")

	log.Print("===> creating ssh session...")
	s := sshSession(h.IPAddress.String, priv)
	log.Print("===> created ssh session...")
	defer s.Close()

	stdin, stdout, stderr := pipes(s)

	go io.Copy(stdin, os.Stdin)
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	// get the current terminal file descriptor
	fd := int(os.Stdin.Fd())

	log.Print("===> creating ssh shell...")
	setupShell(fd, s)
}

// returns a SSH session that can be used to open a remote shell
func sshSession(ip string, priv []byte) *ssh.Session {
	// create the ssh client
	signer, err := ssh.ParsePrivateKey(priv)
	if err != nil {
		log.Fatalf("failed to create a signer for the private key %v", err)
	}

	// generate the ssh config
	sshConfig := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
	}

	ch := make(chan conn)
	address := fmt.Sprintf("%s:65100", ip)
	go func() {
		// trying to connect to ssh here
		attempts := 0 // number of attemps to connect to ssh
		for {
			sshClient, err := ssh.Dial("tcp", address, sshConfig)
			if err == nil {
				ch <- conn{sshClient, nil}
				return
			}
			log.Printf("Waiting for ssh conn to be ready, got %v", err)
			// rejects failed authenticate and connection refused error
			if strings.Contains(err.Error(), "authenticate") ||
				strings.Contains(err.Error(), "connection refused") {
				log.Printf("Failed to authenticate with remote ssh: %v", err)
				attempts += 1
			}
			if attempts < maxAttempts {
				time.Sleep(wait)
				continue
			} else {
				ch <- conn{nil, err}
			}
		}
	}()

	sshConn := <-ch
	if sshConn.err != nil {
		log.Fatalf("failed to ssh into the host %v", sshConn.err)
	}
	c := sshConn.client

	// open a session
	s, err := c.NewSession()
	if err != nil {
		log.Fatalf("failed to create a ssh session %v", err)
	}
	return s
}

// monitor sizes of the current local shell and resize the remote shell as
// necessary
func monitorSize(fd int, s *ssh.Session) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			w, h, err := terminal.GetSize(fd)
			if err != nil {
				log.Printf("failed to get terminal size", err)
				w, h = 80, 24
			}
			req := &resizeRequest{
				Columns: uint32(w),
				Rows:    uint32(h),
				Width:   uint32(w),
				Height:  uint32(h),
			}
			_, err = s.SendRequest("window-change", false, ssh.Marshal(req))
			if err != nil {
				log.Printf("failed to send resize request %v", err)
			}
		}
	}()
}

// setup a SSH shell
func setupShell(fd int, s *ssh.Session) {
	state, err := terminal.GetState(fd)
	if err != nil {
		log.Fatalf("failed to get term state %v", err)
	}
	if _, err = terminal.MakeRaw(fd); err != nil {
		log.Fatalf("failed to make raw terminal %v", err)
	}
	defer terminal.Restore(fd, state)

	// Request pseudo terminal with the current size
	termWidth, termHeight, err := terminal.GetSize(fd)
	if err != nil {
		log.Fatalf("failed to get terminal size %v", err)
	}
	if err := s.RequestPty(termMode, termHeight, termWidth, nil); err != nil {
		log.Fatalf("failed to request a pty via ssh %v", err)
	}

	// Start remote shell
	if err := s.Shell(); err != nil {
		log.Fatalf("failed to open a shell via ssh %v", err)
	}

	// setup resize hook
	go monitorSize(fd, s)

	// wait til session finished
	if err := s.Wait(); err != nil {
		log.Printf("session closed %v", err)
	}
}

// pipes returns stdin, stdout, stderr of the current ssh session
func pipes(s *ssh.Session) (io.WriteCloser, io.Reader, io.Reader) {
	stdin, err := s.StdinPipe()
	if err != nil {
		log.Fatalf("failed to open stdin pipe %v", err)
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		log.Fatalf("failed to open stdout pipe %v", err)
	}
	stderr, err := s.StderrPipe()
	if err != nil {
		log.Fatalf("failed to open stderr pipe %v", err)
	}
	return stdin, stdout, stderr
}
