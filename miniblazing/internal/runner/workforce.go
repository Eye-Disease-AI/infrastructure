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

	"github.com/bitfield/script"
	"krzyzanowski.dev/miniblazing/internal/log"
)

type workforce interface {
	Start() error
	OpenFile(string, int, fs.FileMode) (io.ReadWriteCloser, error)
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
	count   int
	workers []scalewayWorker
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

func (sw *scalewayWorkforce) Stop() error {
	log.Wait("Stopping workers")

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
