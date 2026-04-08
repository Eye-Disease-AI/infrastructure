package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/bitfield/script"
	"krzyzanowski.dev/miniblazing/internal/log"
)

type repository interface {
	readFile(relPath string) (io.ReadCloser, error)
	copyRecursive(relPath string, destPath string) error
}

type gitRepository struct {
	path      string
	commitSha string
}

func NewGitRepository(path string, commitSha string) (*gitRepository, error) {
	checkShaCmd := fmt.Sprintf(
		"git -C %s rev-parse HEAD",
		path,
	)

	realSha, err := script.Exec(checkShaCmd).String()
	if err != nil {
		return nil, err
	}
	realSha = strings.TrimRight(realSha, "\n")

	if realSha != commitSha {
		return nil, errors.New("commit sha mismatch")
	}

	return &gitRepository{
		path:      path,
		commitSha: commitSha,
	}, nil
}

func (gr *gitRepository) ensureInRepo(absPath string) error {
	inRoot, err := isInRoot(gr.path, absPath)
	if err != nil {
		log.Err("Failed to check if file is in repo")
		return err
	}

	if !inRoot {
		log.Err("File '%s' is not in the repo", absPath)
		return err
	}

	return nil
}

func (gr *gitRepository) readFile(relPath string) (io.ReadCloser, error) {
	filePath := path.Join(gr.path, relPath)

	err := gr.ensureInRepo(filePath)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		log.Err("Failed to open '%s'", filePath)
		return nil, err
	}

	return f, nil
}

func (gr *gitRepository) copyRecursive(relPath string, destPath string) error {
	srcPath := path.Join(gr.path, relPath)

	err := gr.ensureInRepo(srcPath)
	if err != nil {
		return err
	}

	// -a makes cp recursive and makes it preserve symlinks instead
	// of following them.
	cpCmd := exec.Command(
		"cp",
		"-a",
		srcPath,
		destPath,
	)

	err = cpCmd.Run()
	if err != nil {
		log.Err(
			"Failed to copy '%s' -> '%s'",
			srcPath,
			destPath,
		)
		return err
	}

	return nil
}
