package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"krzyzanowski.dev/miniblazing/internal/experiment"
	"krzyzanowski.dev/miniblazing/internal/log"
)

const experimentRelPath = "experiment.exp"

var ErrLock error = errors.New("lock error")
var ErrOther error = errors.New("runner error")

var flockPath string

func init() {
	d := thisDir()
	flockPath = path.Join(d, "lock")
}

func RunOn(wf workforce, repo repository) error {
	flock, err := acquireFlock()
	if err != nil {
		log.Err("%s", err)
		return ErrLock
	}
	defer func() {
		_ = flock.Close()
	}()

	exp, err := LoadExperiment(repo)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}

	archivePath, err := prepareUpload(exp, repo)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}

	log.Wait("Starting workforce")
	err = wf.Start()
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	log.Ok("Workforce started")

	log.Wait("Uploading archive")
	arDest, err := wf.OpenFile("/a.tar.gz", os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	arSrc, err := os.Open(archivePath)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	_, err = io.Copy(arDest, arSrc)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	log.Ok("Archive uploaded")

	log.Wait("Running command")
	err = wf.Run("cp /a.tar.gz /b.tar.gz", nil)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	log.Ok("Command finished")

	log.Wait("Downloading artifacts")
	artifactsSrc, err := wf.OpenFile("/b.tar.gz", os.O_RDONLY, 0)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	artifactsDest, err := os.CreateTemp("", "miniblazing-artifacts-*.tar.gz")
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	_, err = io.Copy(artifactsDest, artifactsSrc)
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	log.Ok("Artifacts downloaded @ %s", artifactsDest.Name())

	log.Wait("Stopping workforce")
	err = wf.Stop()
	if err != nil {
		log.Err("%s", err)
		return ErrOther
	}
	log.Ok("Workforce stopped")

	log.Ok("Workload complete")
	return nil
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

func LoadExperiment(repo repository) (*experiment.Experiment, error) {
	log.Wait("Loading experiment config @ %s", experimentRelPath)

	expFile, err := repo.readFile(experimentRelPath)
	if err != nil {
		return nil, err
	}

	exp, err := experiment.Load(expFile)
	if err != nil {
		log.Err("Failed to load experiment config")
		return nil, err
	}

	log.Ok("Experiment loaded = %+v", exp)
	return exp, nil
}

func prepareUpload(
	exp *experiment.Experiment,
	repo repository,
) (string, error) {
	uploadTmpdir, err := os.MkdirTemp("", "miniblazing")
	if err != nil {
		log.Err("Failed to create upload tmpdir")
		return "", err
	}

	log.Wait("Preparing upload archive @ %s", uploadTmpdir)

	for _, upload := range exp.Uploads {
		err := repo.copyRecursive(upload, uploadTmpdir)
		if err != nil {
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

// isInRoot expects paths passed as parameters to be absolute.
func isInRoot(rootPath string, filePath string) (bool, error) {
	relPath, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return false, err
	}

	return filepath.IsLocal(relPath), nil
}
