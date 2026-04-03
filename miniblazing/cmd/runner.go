package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	"krzyzanowski.dev/miniblazing/internal/experiment"
	"krzyzanowski.dev/miniblazing/internal/log"
)

const exitErr = 1
const exitLock = 199

var flockPath string

func init() {
	d := thisDir()
	flockPath = path.Join(d, "lock")
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

		log.Ok("Running workload @ %s#%s", commitSha, repoPath)
		log.Wait("Locking")

		flockFile, err := os.OpenFile(
			flockPath,
			os.O_RDONLY|os.O_CREATE,
			0600,
		)
		if err != nil {
			log.Err("Failed to open flock file")
			os.Exit(exitErr)
		}

		err = syscall.Flock(int(flockFile.Fd()), syscall.LOCK_EX)
		if err != nil {
			log.Err("Failed to acquire lock")
			os.Exit(exitLock)
		}

		log.Ok("Locked")

		configPath := path.Join(repoPath, "experiment.exp")
		log.Wait("Loading experiment config @ %s", configPath)
		exp, err := experiment.LoadFile(configPath)
		if err != nil {
			log.Err("Failed to load experiment config")
			os.Exit(exitErr)
		}

		log.Ok("Experiment loaded = %+v", exp)

		uploadTmpdir, err := os.MkdirTemp("", "miniblazing")
		if err != nil {
			log.Err("Failed to create upload tmpdir")
			os.Exit(exitErr)
		}

		log.Wait("Preparing upload archive @ %s", uploadTmpdir)

		for _, upload := range exp.Uploads {
			uploadRepoPath := path.Join(repoPath, upload)
			log.Wait("Adding %s to archive", uploadRepoPath)
			inRoot, err := isInRoot(repoPath, uploadRepoPath)
			if err != nil {
				log.Err("Failed to check if file is in repo")
				os.Exit(exitErr)
			}

			if !inRoot {
				log.Err("File '%s' is not in the repo", upload)
				os.Exit(exitErr)
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
				os.Exit(exitErr)
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
			os.Exit(exitErr)
		}

		log.Ok("Archive created @ %s", archivePath)

		scwPath := path.Join(thisDir(), "..", "scaleway-host", "scaleway.sh")
		scwPath, err = filepath.Abs(scwPath)
		if err != nil {
			log.Err("Failed to get scaleway helper path")
			os.Exit(exitErr)
		}

		scwCmd := exec.Command(
			scwPath,
			"start",
			strconv.Itoa(exp.NumWorkers),
			"yes",
		)
		scwOut, err := scwCmd.Output()
		if err != nil {
			log.Err("Failed to start workers: %s", err)
			os.Exit(exitErr)
		}

		var startResult struct {
			Workers []struct {
				Pubkey string `json:"pubkey"`
				IpAddr string `json:"ipaddr"`
			} `json:"workers"`
		}

		err = json.Unmarshal(scwOut, &startResult)
		if err != nil {
			log.Err("Failed to unmarshal worker info")
			os.Exit(exitErr)
		}
	}
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
