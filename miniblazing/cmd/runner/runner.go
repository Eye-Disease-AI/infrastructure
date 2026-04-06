package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bitfield/script"
	"krzyzanowski.dev/miniblazing/internal/experiment"
	"krzyzanowski.dev/miniblazing/internal/log"
)

const exitErr = 1
const exitLock = 199

var flockPath string
var scwPath string

type workforce interface {
	start() error
	stop() error
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

func newScalewayWorkforce(count int) *scalewayWorkforce {
	return &scalewayWorkforce{
		count:   count,
		workers: make([]scalewayWorker, 0),
	}
}

func (sw *scalewayWorkforce) start() error {
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

func (sw *scalewayWorkforce) stop() error {
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

func init() {
	var err error

	d := thisDir()
	flockPath = path.Join(d, "lock")

	scwPath, err = filepath.Abs(path.Join(
		thisDir(),
		"..",
		"scaleway-host",
		"scaleway.sh",
	))

	scwPath, err = filepath.Abs(scwPath)
	if err != nil {
		log.Err("Failed to get scaleway helper path")
		os.Exit(exitErr)
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Err("Please provide subcommand")
		return
	}

	switch os.Args[1] {
	case "run_on":
		if len(os.Args) != 4 {
			log.Err("Usage: <commitSha> <repoPath>")
			os.Exit(exitErr)
		}

		commitSha := os.Args[2]
		repoPath := os.Args[3]
		exp, err := loadExperiment(repoPath)
		if err != nil {
			log.Err(err.Error())
			os.Exit(exitErr)
		}

		wf := newScalewayWorkforce(exp.NumWorkers)
		runOn(wf, commitSha, repoPath)
	}
}

func runOn(wf workforce, commitSha string, repoPath string) {
	log.Ok("Running workload @ %s#%s", commitSha, repoPath)

	flock, err := acquireFlock()
	if err != nil {
		log.Err(err.Error())
		os.Exit(exitLock)
	}
	defer flock.Close()

	exp, err := loadExperiment(repoPath)
	if err != nil {
		log.Err(err.Error())
		os.Exit(exitErr)
	}

	_, err = prepareUpload(exp, repoPath)
	if err != nil {
		log.Err(err.Error())
		os.Exit(exitErr)
	}

	err = wf.start()
	if err != nil {
		log.Err(err.Error())
		os.Exit(exitErr)
	}

	err = wf.stop()
	if err != nil {
		log.Err(err.Error())
		os.Exit(exitErr)
	}

	log.Ok("Workload complete")
}

func acquireFlock() (*os.File, error) {
	log.Wait("Locking")

	flockFile, err := os.OpenFile(
		flockPath,
		os.O_RDONLY|os.O_CREATE,
		0600,
	)
	if err != nil {
		log.Err("Failed to open flock file")
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = flockFile.Close()
		}
	}()

	err = syscall.Flock(int(flockFile.Fd()), syscall.LOCK_EX)
	if err != nil {
		log.Err("Failed to acquire lock")
		return nil, err
	}

	log.Ok("Locked")

	return flockFile, nil
}

func loadExperiment(repoPath string) (*experiment.Experiment, error) {
	configPath := path.Join(repoPath, "experiment.exp")

	log.Wait("Loading experiment config @ %s", configPath)

	exp, err := experiment.LoadFile(configPath)
	if err != nil {
		log.Err("Failed to load experiment config")
		return nil, err
	}

	log.Ok("Experiment loaded = %+v", exp)

	return exp, nil
}

func prepareUpload(
	exp *experiment.Experiment,
	repoPath string,
) (string, error) {
	uploadTmpdir, err := os.MkdirTemp("", "miniblazing")
	if err != nil {
		log.Err("Failed to create upload tmpdir")
		return "", err
	}

	log.Wait("Preparing upload archive @ %s", uploadTmpdir)

	for _, upload := range exp.Uploads {
		uploadRepoPath := path.Join(repoPath, upload)
		log.Wait("Adding %s to archive", uploadRepoPath)
		inRoot, err := isInRoot(repoPath, uploadRepoPath)
		if err != nil {
			log.Err("Failed to check if file is in repo")
			return "", err
		}

		if !inRoot {
			log.Err("File '%s' is not in the repo", upload)
			return "", err
		}

		cpCmd := exec.Command(
			"cp",
			"-r",
			uploadRepoPath,
			uploadTmpdir,
		)

		err = cpCmd.Run()
		if err != nil {
			log.Err(
				"Failed to copy '%s' -> '%s'",
				upload,
				uploadTmpdir,
			)
			return "", err
		}
	}

	archivePath := uploadTmpdir + ".tar.gz"
	tarCmd := exec.Command(
		"tar",
		"czvf",
		archivePath,
		uploadTmpdir,
	)

	err = tarCmd.Run()
	if err != nil {
		log.Err(
			"Failed to create upload archive @ %s",
			archivePath,
		)
		return "", err
	}

	log.Ok("Archive created @ %s", archivePath)

	return archivePath, nil
}

func thisDir() string {
	thisPath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return path.Dir(thisPath)
}

func isInRoot(rootPath string, filePath string) (bool, error) {
	relPath, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return false, err
	}

	return filepath.IsLocal(relPath), nil
}
