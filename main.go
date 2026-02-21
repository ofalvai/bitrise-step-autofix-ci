package main

import (
	"fmt"
	"os"

	"github.com/bitrise-steplib/bitrise-step-autofix-ci/step"

	"github.com/bitrise-io/go-steputils/v2/export"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/exitcode"
	"github.com/bitrise-io/go-utils/v2/log"
)

func main() {
	os.Exit(int(run()))
}

func run() exitcode.ExitCode {
	logger := log.NewLogger()
	envRepo := env.NewRepository()
	inputParser := stepconf.NewInputParser(envRepo)
	commandFactory := command.NewFactory(envRepo)
	exporter := export.NewExporter(commandFactory)

	s := step.New(logger, inputParser, commandFactory, envRepo)
	result, err := s.Run()

	// Export outputs regardless of success/failure, so callers can inspect the result.
	exportErr := exportOutputs(exporter, result)
	if exportErr != nil {
		logger.Errorf("Failed to export outputs: %s", exportErr)
	}

	if err != nil {
		logger.Errorf("%s", err)
		return exitcode.Failure
	}

	if result.AutofixNeeded && !result.DryRun {
		// A new build will be triggered by the push; fail this one intentionally
		// so CI gates don't pass on the unfixed commit.
		return exitcode.Failure
	}

	return exitcode.Success
}

func exportOutputs(exporter export.Exporter, result step.Result) error {
	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	if err := exporter.ExportOutput("AUTOFIX_NEEDED", boolStr(result.AutofixNeeded)); err != nil {
		return fmt.Errorf("export AUTOFIX_NEEDED: %w", err)
	}
	if err := exporter.ExportOutput("AUTOFIX_PUSHED", boolStr(result.AutofixPushed)); err != nil {
		return fmt.Errorf("export AUTOFIX_PUSHED: %w", err)
	}
	if err := exporter.ExportOutput("AUTOFIX_FILE_COUNT", fmt.Sprintf("%d", result.FileCount)); err != nil {
		return fmt.Errorf("export AUTOFIX_FILE_COUNT: %w", err)
	}
	return nil
}
