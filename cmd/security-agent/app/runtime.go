// +build linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"

	ddgostatsd "github.com/DataDog/datadog-go/statsd"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/common"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	securityLogger "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cwsIntakeOrigin config.IntakeOrigin = "cloud-workload-security"
)

var (
	activityDumpTags    []string
	activityDumpComm    string
	activityDumpTimeout int
	withGraph           bool
	differentiateArgs   bool
	activityDumpFile    string
)

var (
	runtimeCmd = &cobra.Command{
		Use:   "runtime",
		Short: "Runtime Agent utility commands",
	}

	checkPoliciesCmd = &cobra.Command{
		Use:   "check-policies",
		Short: "Check policies and return a report",
		RunE:  checkPolicies,
	}

	checkPoliciesArgs = struct {
		dir string
	}{}

	dumpCmd = &cobra.Command{
		Use:   "get",
		Short: "Query a resource from the Runtime Security module",
	}

	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop a task of the Runtime Security module",
	}

	listCmd = &cobra.Command{
		Use:   "list",
		Short: "List a resource from the Runtime Security module",
	}

	generateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate command of the Runtime Security module",
	}

	getProcessCacheCmd = &cobra.Command{
		Use:   "process-cache",
		Short: "process cache",
		RunE:  getProcessCache,
	}

	generateActivityDumpCmd = &cobra.Command{
		Use:   "activity-dump",
		Short: "record and dump all activity matching a set of tags or the provided comm",
		RunE:  generateActivityDump,
	}

	listActivityDumpsCmd = &cobra.Command{
		Use:   "activity-dump",
		Short: "list all active dumps",
		RunE:  listActivityDumps,
	}

	stopActivityDumpCmd = &cobra.Command{
		Use:   "activity-dump",
		Short: "stops the first activity dump that matches the provided set of tags",
		RunE:  stopActivityDump,
	}

	generateProfileCmd = &cobra.Command{
		Use:   "profile",
		Short: "generate a profile from an activity dump",
		RunE:  generateProfile,
	}

	selfTestCmd = &cobra.Command{
		Use:   "self-test",
		Short: "Run runtime self test",
		RunE:  runRuntimeSelfTest,
	}
)

func init() {
	generateActivityDumpCmd.Flags().StringArrayVar(
		&activityDumpTags,
		"tags",
		[]string{},
		"tags are used to filter the activity dump in order to select a specific workload. Tags should be provided in the \"tag_name:tag_value\" format.",
	)
	generateActivityDumpCmd.Flags().StringVar(
		&activityDumpComm,
		"comm",
		"",
		"a process command can be used to filter the activity dump from a specific process.",
	)
	generateActivityDumpCmd.Flags().IntVar(
		&activityDumpTimeout,
		"timeout",
		10,
		"timeout for the activity dump in minutes",
	)
	generateActivityDumpCmd.Flags().BoolVar(
		&withGraph,
		"graph",
		false,
		"generate a graph from the generated dump",
	)
	generateActivityDumpCmd.Flags().BoolVar(
		&differentiateArgs,
		"differentiate-args",
		false,
		"add the arguments in the process node merge algorithm",
	)
	stopActivityDumpCmd.Flags().StringArrayVar(
		&activityDumpTags,
		"tags",
		[]string{},
		"tags is used to select an activity dump. Tags should be provided in the [tag_name:tag_value] format.",
	)
	stopActivityDumpCmd.Flags().StringVar(
		&activityDumpComm,
		"comm",
		"",
		"a process command can be used to filter the activity dump from a specific process.",
	)
	generateProfileCmd.Flags().StringVar(
		&activityDumpFile,
		"input",
		"",
		"path to the activity dump file from which a profile will be generated",
	)
	_ = generateProfileCmd.MarkFlagRequired("input")

	dumpCmd.AddCommand(getProcessCacheCmd)
	runtimeCmd.AddCommand(dumpCmd)

	listCmd.AddCommand(listActivityDumpsCmd)
	runtimeCmd.AddCommand(listCmd)

	stopCmd.AddCommand(stopActivityDumpCmd)
	runtimeCmd.AddCommand(stopCmd)

	generateCmd.AddCommand(generateActivityDumpCmd)
	generateCmd.AddCommand(generateProfileCmd)
	runtimeCmd.AddCommand(generateCmd)

	runtimeCmd.AddCommand(checkPoliciesCmd)
	checkPoliciesCmd.Flags().StringVar(&checkPoliciesArgs.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")

	runtimeCmd.AddCommand(selfTestCmd)
}

func getProcessCache(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	rsClient, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer rsClient.Close()

	filename, err := rsClient.DumpProcessCache()
	if err != nil {
		return errors.Wrap(err, "unable to get a process cache dump")
	}

	log.Infof("Process dump file: %s\n", filename)

	return nil
}

func generateActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	rsClient, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer rsClient.Close()

	var filename, graph string
	filename, graph, err = rsClient.GenerateActivityDump(activityDumpTags, activityDumpComm, int32(activityDumpTimeout), withGraph, differentiateArgs)
	if err != nil {
		return errors.Wrap(err, "unable to an request activity dump for %s")
	}

	fmt.Printf("Activity dump file: %s\n", filename)
	if len(graph) > 0 {
		fmt.Printf("Graph dump file: %s\n", graph)
	}

	return nil
}

func listActivityDumps(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	rsClient, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer rsClient.Close()

	var activeDumps []string
	activeDumps, err = rsClient.ListActivityDumps()
	if err != nil {
		return errors.Wrap(err, "unable to request the list activity dumps")
	}

	if len(activeDumps) > 0 {
		fmt.Println("Active dumps:")
		for _, d := range activeDumps {
			fmt.Printf("\t- %s\n", d)
		}
	} else {
		fmt.Println("No active dumps found")
	}

	return nil
}

func stopActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	rsClient, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer rsClient.Close()

	var msg string
	msg, err = rsClient.StopActivityDump(activityDumpTags, activityDumpComm)
	if err != nil {
		return errors.Wrap(err, "unable to stop the request activity dump")
	}

	if len(msg) == 0 {
		fmt.Println("done!")
	} else {
		fmt.Println(msg)
	}

	return nil
}

func generateProfile(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	rsClient, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to generate a profile")
	}
	defer rsClient.Close()

	var output string
	output, err = rsClient.GenerateProfile(activityDumpFile)
	if err != nil {
		return errors.Wrapf(err, "couldn't generate a profile from: %s", activityDumpFile)
	}

	fmt.Printf("Generated profile: %s\n", output)
	return nil
}

func checkPolicies(cmd *cobra.Command, args []string) error {
	cfg := &secconfig.Config{
		PoliciesDir:         checkPoliciesArgs.dir,
		EnableKernelFilters: true,
		EnableApprovers:     true,
		EnableDiscarders:    true,
		PIDCacheSize:        1,
	}

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	opts := rules.NewOptsWithParams(model.SECLConstants, sprobe.SupportedDiscarders, enabled, sprobe.AllCustomRuleIDs(), model.SECLLegacyAttributes, &securityLogger.PatternLogger{})
	model := &model.Model{}
	ruleSet := rules.NewRuleSet(model, model.NewEvent, opts)

	if err := rules.LoadPolicies(cfg.PoliciesDir, ruleSet); err.ErrorOrNil() != nil {
		return err
	}

	approvers, err := ruleSet.GetApprovers(sprobe.GetCapababilities())
	if err != nil {
		return err
	}

	rsa := sprobe.NewRuleSetApplier(cfg, nil)

	report, err := rsa.Apply(ruleSet, approvers)
	if err != nil {
		return err
	}

	content, _ := json.MarshalIndent(report, "", "\t")
	fmt.Printf("%s\n", string(content))

	return nil
}

func runRuntimeSelfTest(cmd *cobra.Command, args []string) error {
	rsClient, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer rsClient.Close()

	selfTestResult, err := rsClient.RunSelfTest()
	if err != nil {
		return errors.Wrap(err, "unable to get a process self test")
	}

	if selfTestResult.Ok {
		fmt.Printf("Runtime self test: OK\n")
	} else {
		fmt.Printf("Runtime self test: error: %v\n", selfTestResult.Error)
	}
	return nil
}

func newRuntimeReporter(stopper restart.Stopper, sourceName, sourceType string, endpoints *config.Endpoints, context *client.DestinationsContext) (event.Reporter, error) {
	health := health.RegisterLiveness("runtime-security")

	// setup the auditor
	auditor := auditor.New(coreconfig.Datadog.GetString("runtime_security_config.run_path"), "runtime-security-registry.json", coreconfig.DefaultAuditorTTL, health)
	auditor.Start()
	stopper.Add(auditor)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, &diagnostic.NoopMessageReceiver{}, nil, endpoints, context)
	pipelineProvider.Start()
	stopper.Add(pipelineProvider)

	logSource := config.NewLogSource(
		sourceName,
		&config.LogsConfig{
			Type:   sourceType,
			Source: sourceName,
		},
	)
	return event.NewReporter(logSource, pipelineProvider.NextPipelineChan()), nil
}

// This function will only be used on Linux. The only platforms where the runtime agent runs
func newLogContextRuntime() (*config.Endpoints, *client.DestinationsContext, error) { // nolint: deadcode, unused
	logsConfigComplianceKeys := config.NewLogsConfigKeys("runtime_security_config.endpoints.", coreconfig.Datadog)
	return newLogContext(logsConfigComplianceKeys, "runtime-security-http-intake.logs.", "logs", cwsIntakeOrigin, config.DefaultIntakeProtocol)
}

func startRuntimeSecurity(hostname string, stopper restart.Stopper, statsdClient *ddgostatsd.Client) (*secagent.RuntimeSecurityAgent, error) {
	enabled := coreconfig.Datadog.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil, nil
	}

	endpoints, context, err := newLogContextRuntime()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(context)

	reporter, err := newRuntimeReporter(stopper, "runtime-security-agent", "runtime-security", endpoints, context)
	if err != nil {
		return nil, err
	}

	agent, err := secagent.NewRuntimeSecurityAgent(hostname, reporter)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create a runtime security agent instance")
	}
	agent.Start()

	stopper.Add(agent)

	log.Info("Datadog runtime security agent is now running")

	return agent, nil
}
