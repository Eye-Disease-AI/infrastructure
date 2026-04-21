package runner

import (
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
)

//go:embed testdata/example.exp
var testExp string

func TestBasic(t *testing.T) {
	tr, err := newTestRepository()
	if err != nil {
		t.Error(err)
		return
	}

	wf := newTestWorkforce()
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

			_, err = io.Copy(destFile, srcFile)
			if err != nil {
				return err
			}

			fmt.Println("Copied file")
		}
	}

	return nil
}

type testWorkforce struct {
	fs billy.Filesystem
}

func newTestWorkforce() testWorkforce {
	return testWorkforce{
		fs: memfs.New(),
	}
}

func (testWorkforce) Start() error {
	return nil
}

func (tw testWorkforce) OpenFile(
	filename string,
	flag int,
	perm fs.FileMode,
) ([]io.ReadWriteCloser, error) {
	f, err := tw.fs.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	return []io.ReadWriteCloser{f}, nil
}

type testCommandFn func(fs billy.Filesystem, args []string, stdin <-chan string) error

var testCommands = map[string]testCommandFn{
	"cp": func(bfs billy.Filesystem, args []string, stdin <-chan string) error {
		if len(args) != 2 {
			return fmt.Errorf("cp: expected 2 arguments, got %d", len(args))
		}
		src, err := bfs.Open(args[0])
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()

		srcInfo, err := src.Stat()
		if err != nil {
			return err
		}

		dst, err := bfs.OpenFile(args[1], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
		if err != nil {
			return err
		}
		defer func() { _ = dst.Close() }()

		_, err = io.Copy(dst, src)
		return err
	},
}

func (tw testWorkforce) Run(cmd string, stdin <-chan string) error {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}
	handler, ok := testCommands[parts[0]]
	if !ok {
		return fmt.Errorf("unknown command: %s", parts[0])
	}
	return handler(tw.fs, parts[1:], stdin)
}

func (testWorkforce) Stop() error {
	return nil
}
