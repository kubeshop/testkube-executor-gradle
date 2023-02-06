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
	"github.com/kubeshop/testkube/pkg/executor/runner"
	"github.com/kubeshop/testkube/pkg/executor/secret"
	"github.com/kubeshop/testkube/pkg/ui"
)

type Params struct {
	Datadir string // RUNNER_DATADIR
}

func NewRunner() *GradleRunner {
	output.PrintLog(fmt.Sprintf("%s Preparing test runner", ui.IconTruck))

	output.PrintLog(fmt.Sprintf("%s Reading environment variables...", ui.IconWorld))
	params := Params{
		Datadir: os.Getenv("RUNNER_DATADIR"),
	}
	output.PrintLog(fmt.Sprintf("%s Environment variables read successfully", ui.IconCheckMark))
	output.PrintLog(fmt.Sprintf("RUNNER_DATADIR=\"%s\"", params.Datadir))

	runner := &GradleRunner{
		params: params,
	}

	return runner
}

type GradleRunner struct {
	params Params
}

func (r *GradleRunner) Run(execution testkube.Execution) (result testkube.ExecutionResult, err error) {
	output.PrintLog(fmt.Sprintf("%s Preparing for test run", ui.IconTruck))

	// check that the datadir exists
	_, err = os.Stat(r.params.Datadir)
	if errors.Is(err, os.ErrNotExist) {
		output.PrintLog(fmt.Sprintf("%s Datadir %s does not exist", ui.IconCross, r.params.Datadir))
		return result, err
	}

	// TODO design it better for now just append variables as envs
	envManager := secret.NewEnvManagerWithVars(execution.Variables)
	envManager.GetVars(envManager.Variables)
	for _, env := range envManager.Variables {
		os.Setenv(env.Name, env.Value)
	}

	// the Gradle executor does not support files
	if execution.Content.IsFile() {
		output.PrintLog(fmt.Sprintf("%s executor only supports git-dir based tests", ui.IconCross))
		return *result.Err(fmt.Errorf("executor only supports git-dir based tests")), nil
	}

	// check settings.gradle or settings.gradle.kts files exist
	directory := filepath.Join(r.params.Datadir, "repo", execution.Content.Repository.Path)

	settingsGradle := filepath.Join(directory, "settings.gradle")
	settingsGradleKts := filepath.Join(directory, "settings.gradle.kts")

	_, settingsGradleErr := os.Stat(settingsGradle)
	_, settingsGradleKtsErr := os.Stat(settingsGradleKts)
	if errors.Is(settingsGradleErr, os.ErrNotExist) && errors.Is(settingsGradleKtsErr, os.ErrNotExist) {
		output.PrintLog(fmt.Sprintf("%s no settings.gradle or settings.gradle.kts found", ui.IconCross))
		return *result.Err(fmt.Errorf("no settings.gradle or settings.gradle.kts found")), nil
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

	task := strings.Split(execution.TestType, "/")[1]
	if !strings.EqualFold(task, "project") {
		// then use the test subtype as task name
		args = append(args, task)
	}

	runPath := directory
	if execution.Content.Repository != nil && execution.Content.Repository.WorkingDir != "" {
		runPath = filepath.Join(r.params.Datadir, "repo", execution.Content.Repository.WorkingDir)
		args = append(args, "-p", directory)
	}

	output.PrintEvent("Running task: "+task, directory, gradleCommand, args)
	out, err := executor.Run(runPath, gradleCommand, envManager, args...)
	out = envManager.Obfuscate(out)

	ls := []string{}
	filepath.Walk("/data", func(path string, info fs.FileInfo, err error) error {
		ls = append(ls, path)
		return nil
	})
	output.PrintEvent("/data content", ls)

	if err == nil {
		output.PrintLog(fmt.Sprintf("%s Test execution passed", ui.IconCheckMark))
		result.Status = testkube.ExecutionStatusPassed
	} else {
		output.PrintLog(fmt.Sprintf("%s Test execution failed: %s", ui.IconCross, err.Error()))
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
			output.PrintLog(fmt.Sprintf("%s Could not process reports: %s", ui.IconCross, err.Error()))
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
		return *result.Err(err), nil
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

// GetType returns runner type
func (r *GradleRunner) GetType() runner.Type {
	return runner.TypeMain
}
