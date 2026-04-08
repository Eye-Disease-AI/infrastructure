package main

import (
	"errors"
	"os"

	"krzyzanowski.dev/miniblazing/internal/log"
	"krzyzanowski.dev/miniblazing/internal/runner"
)

const exitErr = 1
const exitLock = 199

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

		repo, err := runner.NewGitRepository(repoPath, commitSha)
		if err != nil {
			log.Err("%s", err)
			os.Exit(exitErr)
		}

		exp, err := runner.LoadExperiment(repo)
		if err != nil {
			log.Err("%s", err)
			os.Exit(exitErr)
		}

		log.Ok("Running workload @ %s#%s", commitSha, repoPath)
		wf := runner.NewScalewayWorkforce(exp.NumWorkers)

		err = runner.RunOn(wf, repo)
		if errors.Is(err, runner.ErrLock) {
			os.Exit(exitLock)
		} else if err != nil {
			os.Exit(exitErr)
		}
	}
}
