package runner

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	junit "github.com/joshdk/go-junit"
	"github.com/kubeshop/testkube/pkg/api/v1/testkube"
	"github.com/kubeshop/testkube/pkg/executor"
	"github.com/kubeshop/testkube/pkg/executor/output"
)

type Params struct {
	Datadir string // RUNNER_DATADIR
}

func NewRunner() *GradleRunner {
	params := Params{
		Datadir: os.Getenv("RUNNER_DATADIR"),
	}

	runner := &GradleRunner{
		params: params,
	}

	return runner
}

type GradleRunner struct {
	params Params
}

func (r *GradleRunner) Run(execution testkube.Execution) (result testkube.ExecutionResult, err error) {
	// check that the datadir exists
	_, err = os.Stat(r.params.Datadir)
	if errors.Is(err, os.ErrNotExist) {
		return result, err
	}

	// the Gradle executor does not support files
	if execution.Content.IsFile() {
		return result.Err(fmt.Errorf("executor only support git-dir based tests")), nil
	}

	// check settings.gradle or settings.gradle.kts files exist
	directory := filepath.Join(r.params.Datadir, "repo", execution.Content.Repository.Path)

	settingsGradle := filepath.Join(directory, "settings.gradle")
	settingsGradleKts := filepath.Join(directory, "settings.gradle.kts")

	_, settingsGradleErr := os.Stat(settingsGradle)
	_, settingsGradleKtsErr := os.Stat(settingsGradleKts)
	if errors.Is(settingsGradleErr, os.ErrNotExist) && errors.Is(settingsGradleKtsErr, os.ErrNotExist) {
		return result.Err(fmt.Errorf("no settings.gradle or settings.gradle.kts found")), nil
	}

	// determine the Gradle command to use
	gradleCommand := "gradle"
	gradleWrapper := filepath.Join(directory, "gradlew")
	_, err = os.Stat(gradleWrapper)
	if err == nil {
		// then we use the wrapper instead
		gradleCommand = "./gradlew"
	}

	// simply set the ENVs to use during Gradle execution
	for key, value := range execution.Envs {
		os.Setenv(key, value)
	}

	// pass additional executor arguments/flags to Gradle
	args := []string{"--no-daemon"}
	args = append(args, execution.Args...)

	task := ""
	if !strings.EqualFold(execution.TestType, "gradle/project") {
		// then use the test subtype as task name
		task = strings.Split(execution.TestType, "/")[1]
		args = append(args, task)
	}

	output.PrintEvent("Running", directory, gradleCommand, args)
	out, err := executor.Run(directory, gradleCommand, args...)

	ls := []string{}
	filepath.Walk("/data", func(path string, info fs.FileInfo, err error) error {
		ls = append(ls, path)
		return nil
	})
	output.PrintEvent("/data content", ls)

	if err == nil {
		result.Status = testkube.ExecutionStatusPassed
	} else {
		result.Status = testkube.ExecutionStatusFailed
		result.ErrorMessage = err.Error()
		if strings.Contains(result.ErrorMessage, "exit status 1") {
			// probably some tests have failed
			result.ErrorMessage = "build failed with an exception"
		} else {
			// Gradle was unable to run at all
			return result, nil
		}
	}

	result.Output = string(out)
	result.OutputType = "text/plain"

	junitReportPath := filepath.Join(directory, "build", "test-results")
	err = filepath.Walk(junitReportPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".xml" {
			suites, _ := junit.IngestFile(path)
			for _, suite := range suites {
				for _, test := range suite.Tests {
					result.Steps = append(
						result.Steps,
						testkube.ExecutionStepResult{
							Name:     fmt.Sprintf("%s - %s", suite.Name, test.Name),
							Duration: test.Duration.String(),
							Status:   mapStatus(test.Status),
						})
				}
			}
		}

		return nil
	})

	if err != nil {
		return result.Err(err), nil
	}

	return result, nil
}

func mapStatus(in junit.Status) (out string) {
	switch string(in) {
	case "passed":
		return string(testkube.PASSED_ExecutionStatus)
	default:
		return string(testkube.FAILED_ExecutionStatus)
	}
}
