package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bitfield/script"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"krzyzanowski.dev/miniblazing/internal/log"
)

type workforce interface {
	Start() error
	OpenFile(string, int, fs.FileMode) ([]io.ReadWriteCloser, error)
	Run(string, <-chan string) error
	Stop() error
}

type scalewayWorker struct {
	Pubkey string `json:"pubkey"`
	IpAddr string `json:"ipaddr"`
}

type scalewayStartResult struct {
	Workers []scalewayWorker `json:"workers"`
}

type scalewayWorkforce struct {
	count       int
	workers     []scalewayWorker
	sshClients  []*ssh.Client
	sftpClients []*sftp.Client
}

var scwPath string

func init() {
	var err error

	scwPath, err = filepath.Abs(path.Join(
		thisDir(),
		"..",
		"scaleway-host",
		"scaleway.sh",
	))
	if err != nil {
		log.Err("Failed to absolutized scaleway helper path")
		panic(err)
	}

	scwPath, err = filepath.Abs(scwPath)
	if err != nil {
		log.Err("Failed to get scaleway helper path")
		panic(err)
	}
}

func NewScalewayWorkforce(count int) *scalewayWorkforce {
	return &scalewayWorkforce{
		count:   count,
		workers: make([]scalewayWorker, 0),
	}
}

func (sw *scalewayWorkforce) Start() error {
	currentEnv := os.Environ()
	newEnv := append(currentEnv, "LOG_DEST=stderr")
	scwOut := bytes.NewBuffer([]byte{})

	startCmd := fmt.Sprintf(
		"%s start %d yes",
		scwPath,
		sw.count,
	)
	log.Info("Start cmd: %s", startCmd)
	_, err := script.
		Exec(startCmd).
		WithEnv(newEnv).
		WithStdout(scwOut).
		WithStderr(os.Stderr).
		Stdout()
	if err != nil {
		log.Err("Failed to start workers: %s", err)
		return err
	}

	log.Ok("Workers started")
	log.Info("Workers JSON: %s", scwOut.String())

	var ssr scalewayStartResult
	err = json.Unmarshal(scwOut.Bytes(), &ssr)
	if err != nil {
		log.Err("Failed to unmarshal worker info")
		return err
	}

	sw.workers = ssr.Workers
	sw.sshClients = make([]*ssh.Client, len(sw.workers))
	sw.sftpClients = make([]*sftp.Client, len(sw.workers))
	log.Info("Workforce started: %+v", ssr)

	return nil
}

type remoteFile struct {
	r       io.Reader
	w       io.Writer
	closeFn func() error
}

func (f *remoteFile) Read(p []byte) (int, error) {
	if f.r == nil {
		return 0, errors.New("not readable")
	}
	return f.r.Read(p)
}

func (f *remoteFile) Write(p []byte) (int, error) {
	if f.w == nil {
		return 0, errors.New("not writable")
	}
	return f.w.Write(p)
}

func (f *remoteFile) Close() error { return f.closeFn() }

func (sw *scalewayWorkforce) sshClient(i int) (*ssh.Client, error) {
	if sw.sshClients[i] != nil {
		return sw.sshClients[i], nil
	}

	ip := sw.workers[i].IpAddr

	key, err := os.ReadFile(os.ExpandEnv("$HOME/.ssh/id_rsa"))
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	c, err := ssh.Dial("tcp", ip+":22", &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return nil, err
	}

	sw.sshClients[i] = c
	return c, nil
}

func (sw *scalewayWorkforce) sftpClient(i int) (*sftp.Client, error) {
	if sw.sftpClients[i] != nil {
		return sw.sftpClients[i], nil
	}

	sc, err := sw.sshClient(i)
	if err != nil {
		return nil, err
	}

	c, err := sftp.NewClient(sc)
	if err != nil {
		return nil, err
	}

	sw.sftpClients[i] = c
	return c, nil
}

func (sw *scalewayWorkforce) Run(cmd string, _ <-chan string) error {
	errs := make([]error, len(sw.workers))
	var wg sync.WaitGroup
	for i := range sw.workers {
		wg.Go(func() {
			sc, err := sw.sshClient(i)
			if err != nil {
				errs[i] = err
				return
			}
			sess, err := sc.NewSession()
			if err != nil {
				errs[i] = err
				return
			}
			defer sess.Close()
			sess.Stdout = os.Stdout
			sess.Stderr = os.Stderr
			errs[i] = sess.Run(cmd)
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (sw *scalewayWorkforce) OpenFile(
	filename string,
	flag int,
	perm fs.FileMode,
) ([]io.ReadWriteCloser, error) {
	if len(sw.workers) == 0 {
		return nil, errors.New("no workers available")
	}

	files := make([]io.ReadWriteCloser, len(sw.workers))
	for i := range sw.workers {
		c, err := sw.sftpClient(i)
		if err != nil {
			for _, f := range files[:i] {
				f.Close()
			}
			return nil, err
		}
		f, err := c.OpenFile(filename, flag)
		if err != nil {
			for _, f := range files[:i] {
				f.Close()
			}
			return nil, err
		}
		files[i] = f
	}
	return files, nil
}

func (sw *scalewayWorkforce) Stop() error {
	log.Wait("Stopping workers")

	for _, c := range sw.sftpClients {
		if c != nil {
			c.Close()
		}
	}
	for _, c := range sw.sshClients {
		if c != nil {
			c.Close()
		}
	}

	pubkeys := []string{}
	for _, worker := range sw.workers {
		pubkeys = append(pubkeys, worker.Pubkey)
	}
	pubkeysArr := strings.Join(pubkeys, " ")

	stopCmd := fmt.Sprintf(
		"%s stop %s",
		scwPath,
		pubkeysArr,
	)
	log.Info("Stop cmd: %s", stopCmd)
	_, err := script.
		Exec(stopCmd).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Stdout()
	if err != nil {
		return err
	}

	log.Ok("Workforce stopped")
	return nil
}
