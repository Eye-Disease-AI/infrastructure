package runner

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
	"krzyzanowski.dev/miniblazing/internal/log"
)

//go:embed testdata/example.exp
var testExp string

func TestBasic(t *testing.T) {
	tr, err := newTestRepository()
	if err != nil {
		t.Error(err)
		return
	}

	wf := testWorkforce{}
	err = RunOn(wf, tr)
	if err != nil {
		t.Error(err)
	}
}

type testRepository struct {
	worktreeFs billy.Filesystem
	gitStorer  memory.Storage
}

func newTestRepository() (*testRepository, error) {
	worktreeFs := memfs.New()
	gitStorer := memory.NewStorage()

	imr, err := git.Init(gitStorer, git.WithWorkTree(worktreeFs))
	if err != nil {
		return nil, err
	}

	f, err := worktreeFs.OpenFile(
		"experiment.exp",
		os.O_CREATE|os.O_RDWR,
		0666,
	)
	if err != nil {
		return nil, err
	}

	_, _ = f.Write([]byte(testExp))
	_ = f.Close()

	f, err = worktreeFs.Create("test1")
	if err != nil {
		return nil, err
	}
	_ = f.Close()

	worktree, err := imr.Worktree()
	if err != nil {
		return nil, err
	}

	_, err = worktree.Add("experiment.exp")
	if err != nil {
		return nil, err
	}

	_, err = worktree.Add("test1")
	if err != nil {
		return nil, err
	}

	commit, err := worktree.Commit("Some commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Yo yo",
			Email: "yo@yo",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, err
	}

	fmt.Println(commit)
	fmt.Println(imr.Head())

	return &testRepository{
		worktreeFs: worktreeFs,
		gitStorer:  *gitStorer,
	}, nil
}

func (tr *testRepository) readFile(
	relPath string,
) (io.ReadCloser, error) {
	return tr.worktreeFs.Open(relPath)
}

func (tr *testRepository) copyRecursive(
	relPath string,
	destPath string,
) error {
	des, err := tr.worktreeFs.ReadDir(relPath)
	if err != nil {
		return err
	}

	dirStat, err := tr.worktreeFs.Lstat(tr.worktreeFs.Root())
	if err != nil {
		return err
	}

	err = os.MkdirAll(destPath, dirStat.Mode())
	if err != nil {
		return err
	}

	for _, de := range des {
		fmt.Println(de)

		if de.IsDir() {
			fullPath := path.Join(relPath, de.Name())
			subDestPath := path.Join(destPath, de.Name())
			err = tr.copyRecursive(fullPath, subDestPath)
			if err != nil {
				return err
			}

			fmt.Println("Copied dir")
		} else {
			srcFilePath := path.Join(relPath, de.Name())
			srcFile, err := tr.worktreeFs.Open(srcFilePath)
			if err != nil {
				return err
			}

			srcFileStat, err := srcFile.Stat()
			if err != nil {
				return err
			}

			destFilePath := path.Join(destPath, de.Name())
			destFile, err := os.OpenFile(destFilePath, os.O_CREATE|os.O_RDWR, srcFileStat.Mode())
			if err != nil {
				return err
			}

			_, err = io.Copy(srcFile, destFile)
			if err != nil {
				return err
			}

			fmt.Println("Copied file")
		}
	}

	return nil
}

type testWorkforce struct{}

func (testWorkforce) Start() error {
	log.Info("Starting test workforce")
	return nil
}

func (testWorkforce) Stop() error {
	log.Info("Stopping test workforce")
	return nil
}
