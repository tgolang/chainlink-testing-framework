package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-testing-framework/tools/flakeguard/reports"
	"github.com/spf13/cobra"
)

var AggregateResultsCmd = &cobra.Command{
	Use:   "aggregate-results",
	Short: "Aggregate test results into a single JSON report",
	Run: func(cmd *cobra.Command, args []string) {
		fs := reports.OSFileSystem{}

		// Get flag values
		resultsPath, _ := cmd.Flags().GetString("results-path")
		outputDir, _ := cmd.Flags().GetString("output-path")
		maxPassRatio, _ := cmd.Flags().GetFloat64("max-pass-ratio")
		codeOwnersPath, _ := cmd.Flags().GetString("codeowners-path")
		repoPath, _ := cmd.Flags().GetString("repo-path")
		repoURL, _ := cmd.Flags().GetString("repo-url")
		branchName, _ := cmd.Flags().GetString("branch-name")
		headSHA, _ := cmd.Flags().GetString("head-sha")
		baseSHA, _ := cmd.Flags().GetString("base-sha")
		githubWorkflowName, _ := cmd.Flags().GetString("github-workflow-name")
		githubWorkflowRunURL, _ := cmd.Flags().GetString("github-workflow-run-url")
		reportID, _ := cmd.Flags().GetString("report-id")

		initialDirSize, err := getDirSize(resultsPath)
		if err != nil {
			log.Error().Err(err).Str("path", resultsPath).Msg("Error getting initial directory size")
			// intentionally don't exit here, as we can still proceed with the aggregation
		}

		// Ensure the output directory exists
		if err := fs.MkdirAll(outputDir, 0755); err != nil {
			log.Error().Err(err).Str("path", outputDir).Msg("Error creating output directory")
			os.Exit(ErrorExitCode)
		}

		// Start spinner for loading test reports
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Suffix = " Aggregating test reports..."
		s.Start()
		fmt.Println()

		// Load test reports from JSON files and aggregate them
		aggregatedReport, err := reports.LoadAndAggregate(
			resultsPath,
			reports.WithRepoPath(repoPath),
			reports.WithCodeOwnersPath(codeOwnersPath),
			reports.WithReportID(reportID),
			reports.WithBranchName(branchName),
			reports.WithBaseSha(baseSHA),
			reports.WithHeadSha(headSHA),
			reports.WithRepoURL(repoURL),
			reports.WithGitHubWorkflowName(githubWorkflowName),
			reports.WithGitHubWorkflowRunURL(githubWorkflowRunURL),
		)
		if err != nil {
			s.Stop()
			log.Error().Err(err).Stack().Msg("Error aggregating test reports")
			os.Exit(ErrorExitCode)
		}
		s.Stop()
		log.Debug().Msg("Successfully loaded and aggregated test reports")

		// Start spinner for mapping test results to paths
		s = spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Suffix = " Filter failed tests..."
		s.Start()

		failedTests := reports.FilterTests(aggregatedReport.Results, func(tr reports.TestResult) bool {
			return !tr.Skipped && tr.PassRatio < maxPassRatio
		})
		s.Stop()

		// Check if there are any failed tests
		if len(failedTests) > 0 {
			log.Info().Int("count", len(failedTests)).Msg("Found failed tests")

			// Create a new report for failed tests with logs
			failedReportWithLogs := &reports.TestReport{
				GoProject:          aggregatedReport.GoProject,
				SummaryData:        aggregatedReport.SummaryData,
				RaceDetection:      aggregatedReport.RaceDetection,
				ExcludedTests:      aggregatedReport.ExcludedTests,
				SelectedTests:      aggregatedReport.SelectedTests,
				HeadSHA:            aggregatedReport.HeadSHA,
				BaseSHA:            aggregatedReport.BaseSHA,
				GitHubWorkflowName: aggregatedReport.GitHubWorkflowName,
				Results:            failedTests,
			}

			// Save the failed tests report with logs
			failedTestsReportWithLogsPath := filepath.Join(outputDir, "failed-test-results-with-logs.json")
			if err := reports.SaveReport(fs, failedTestsReportWithLogsPath, *failedReportWithLogs); err != nil {
				log.Error().Stack().Err(err).Msg("Error saving failed tests report with logs")
				os.Exit(ErrorExitCode)
			}
			log.Debug().Str("path", failedTestsReportWithLogsPath).Msg("Failed tests report with logs saved")

			// Remove logs from test results for the report without logs
			for i := range failedReportWithLogs.Results {
				failedReportWithLogs.Results[i].PassedOutputs = nil
				failedReportWithLogs.Results[i].FailedOutputs = nil
				failedReportWithLogs.Results[i].PackageOutputs = nil
			}

			// Save the failed tests report without logs
			failedTestsReportNoLogsPath := filepath.Join(outputDir, "failed-test-results.json")
			if err := reports.SaveReport(fs, failedTestsReportNoLogsPath, *failedReportWithLogs); err != nil {
				log.Error().Stack().Err(err).Msg("Error saving failed tests report without logs")
				os.Exit(ErrorExitCode)
			}
			log.Debug().Str("path", failedTestsReportNoLogsPath).Msg("Failed tests report without logs saved")
		} else {
			log.Debug().Msg("No failed tests found. Skipping generation of failed tests reports")
		}

		// Remove logs from test results for the aggregated report
		for i := range aggregatedReport.Results {
			aggregatedReport.Results[i].PassedOutputs = nil
			aggregatedReport.Results[i].FailedOutputs = nil
			aggregatedReport.Results[i].PackageOutputs = nil
		}

		// Save the aggregated report to the output directory
		aggregatedReportPath := filepath.Join(outputDir, "all-test-results.json")
		if err := reports.SaveReport(fs, aggregatedReportPath, *aggregatedReport); err != nil {
			log.Error().Stack().Err(err).Msg("Error saving aggregated test report")
			os.Exit(ErrorExitCode)
		}

		finalDirSize, err := getDirSize(resultsPath)
		if err != nil {
			log.Error().Err(err).Str("path", resultsPath).Msg("Error getting final directory size")
			// intentionally don't exit here, as we can still proceed with the aggregation
		}
		diskSpaceUsed := byteCountSI(finalDirSize - initialDirSize)
		log.Info().Str("disk space used", diskSpaceUsed).Str("report", aggregatedReportPath).Msg("Aggregation complete")
	},
}

func init() {
	AggregateResultsCmd.Flags().StringP("results-path", "p", "", "Path to the folder containing JSON test result files (required)")
	AggregateResultsCmd.Flags().StringP("output-path", "o", "./report", "Path to output the aggregated results (directory)")
	AggregateResultsCmd.Flags().Float64P("max-pass-ratio", "", 1.0, "The maximum pass ratio threshold for a test to be considered flaky")
	AggregateResultsCmd.Flags().StringP("codeowners-path", "", "", "Path to the CODEOWNERS file")
	AggregateResultsCmd.Flags().StringP("repo-path", "", ".", "The path to the root of the repository/project")
	AggregateResultsCmd.Flags().String("repo-url", "", "The repository URL")
	AggregateResultsCmd.Flags().String("branch-name", "", "Branch name for the test report")
	AggregateResultsCmd.Flags().String("head-sha", "", "Head commit SHA for the test report")
	AggregateResultsCmd.Flags().String("base-sha", "", "Base commit SHA for the test report")
	AggregateResultsCmd.Flags().String("github-workflow-name", "", "GitHub workflow name for the test report")
	AggregateResultsCmd.Flags().String("github-workflow-run-url", "", "GitHub workflow run URL for the test report")
	AggregateResultsCmd.Flags().String("report-id", "", "Optional identifier for the test report. Will be generated if not provided")

	if err := AggregateResultsCmd.MarkFlagRequired("results-path"); err != nil {
		log.Fatal().Err(err).Msg("Error marking flag as required")
	}
}

// getDirSize returns the size of a directory in bytes
// helpful for tracking how much data is being produced on disk
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// byteCountSI returns a human-readable byte count (decimal SI units)
func byteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}
