package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/sandbox"
)

func (a *App) sandboxCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent sandbox validate|template")
	}
	switch args[0] {
	case "validate":
		fs := newFlagSet("sandbox validate", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent sandbox validate <manifest.json>")
		}
		manifest, err := readSandboxManifest(fs.Arg(0))
		if err != nil {
			return err
		}
		validated, err := sandbox.NewNoopRunner().ValidateManifest(ctx, manifest)
		if err != nil {
			return err
		}
		fingerprint, err := validated.Fingerprint()
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "valid: true\nprotocol: %s\nbackend: %s\nmanifest_fingerprint: %s\narguments: %d\nmounts: %d\nwritable_mounts: %d\nenvironment_bindings: %d\nsecret_references: %d\nnetwork_mode: %s\nallowed_targets: %d\ninput_artifacts: %d\noutputs: %d\ntimeout_seconds: %d\nvalidator: noop\nbackend_enabled: false\nexecution_authorized: false\n",
			validated.ProtocolVersion, validated.Backend, fingerprint,
			len(validated.Command.Arguments), len(validated.Mounts), validated.WritableMountCount(),
			len(validated.Environment), validated.SecretReferenceCount(),
			validated.Network.Mode, len(validated.Network.AllowedTargets),
			len(validated.InputArtifactIDs), validated.OutputCount(), validated.TimeoutSeconds)
		return nil
	case "template":
		fs := newFlagSet("sandbox template", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("usage: cyberagent sandbox template")
		}
		encoded, err := json.MarshalIndent(defaultSandboxManifestTemplate(), "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, string(encoded))
		return nil
	default:
		return fmt.Errorf("unknown sandbox subcommand %q", args[0])
	}
}

func (a *App) runSandboxManifest(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cyberagent run sandbox prepare|list|show|request|review|candidate|candidates|candidate-show|begin|preflight|preflights|preflight-show|evidence|evidences|evidence-show|output-simulate|output-simulations|output-simulation-show|observe|observations|observation-show|docker-plan|docker-plans|docker-plan-show|docker-rehearse|docker-rehearsals|docker-rehearsal-show|cancel|cleanup|executions|execution-show")
	}
	service := application.NewSandboxManifestService(a.store, a.checker)
	if a.dockerObserver != nil {
		service.WithDockerProductionObserver(a.dockerObserver)
	}
	if a.dockerWriteTransport != nil {
		service.WithDockerContainerWriteTransport(a.dockerWriteTransport)
	}
	switch args[0] {
	case "prepare":
		fs := newFlagSet("run sandbox prepare", a.errOut)
		manifestPath := fs.String("manifest", "", "sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable sandbox preparation operation key")
		approvalID := fs.String("approval", "", "exact sandbox approval identity")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "approval": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox prepare <run-id> --manifest <manifest.json> --operation-key <key> [--approval <id>] [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		result, prepareErr := service.Prepare(ctx, application.PrepareSandboxManifestRequest{
			RunID: fs.Arg(0), Manifest: manifest, ApprovalID: *approvalID,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
		if result.Preparation.ID != "" {
			printSandboxManifestIntent(a, result)
		}
		return prepareErr
	case "list":
		fs := newFlagSet("run sandbox list", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox preparations")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox list <run-id> [--limit <n>]")
		}
		values, err := service.List(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox manifest preparations")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tbackend=%s\tpolicy_allowed=%t\tapproval=%s\texecution_authorized=false\tprepared_at=%s\n",
				value.Preparation.ID, value.Preparation.Backend,
				value.Validation.PolicyAllowed, value.Validation.ApprovalStatus,
				value.Preparation.PreparedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "show":
		fs := newFlagSet("run sandbox show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox show <preparation-id>")
		}
		value, err := service.Get(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxManifestIntent(a, value)
		return nil
	case "request":
		fs := newFlagSet("run sandbox request", a.errOut)
		operator := fs.String("operator", "cli_operator", "approval requester identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"operator": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox request <preparation-id> [--operator <id>]")
		}
		record, err := service.RequestApproval(ctx, fs.Arg(0), *operator)
		if err != nil {
			return err
		}
		printApproval(a.out, record)
		return nil
	case "review":
		fs := newFlagSet("run sandbox review", a.errOut)
		decision := fs.String("decision", "", "approve or deny")
		operationKey := fs.String("operation-key", "", "stable approval review operation key")
		reviewer := fs.String("reviewer", "cli_operator", "reviewer identity")
		reason := fs.String("reason", "", "required denial reason")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"decision": true, "operation-key": true, "reviewer": true, "reason": true,
		})); err != nil {
			return err
		}
		action := approval.Action(strings.TrimSpace(*decision))
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" ||
			(action != approval.ActionApprove && action != approval.ActionDeny) {
			return errors.New("usage: cyberagent run sandbox review <preparation-id> --decision approve|deny --operation-key <key> [--reviewer <id>] [--reason <text>]")
		}
		result, err := service.ReviewApproval(ctx, fs.Arg(0), action, *operationKey,
			*reviewer, *reason)
		if err != nil {
			return err
		}
		printApproval(a.out, result.Approval)
		fmt.Fprintf(a.out, "replayed: %t\n", result.Replayed)
		return nil
	case "candidate":
		fs := newFlagSet("run sandbox candidate", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable candidate validation operation key")
		approvalID := fs.String("approval", "", "exact approved sandbox approval identity")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "approval": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox candidate <preparation-id> --manifest <manifest.json> --operation-key <key> [--approval <id>] [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		result, err := service.ValidateExecutionCandidate(ctx,
			application.ValidateSandboxExecutionCandidateRequest{
				PreparationID: fs.Arg(0), Manifest: manifest, ApprovalID: *approvalID,
				OperationKey: *operationKey, RequestedBy: *operator,
			})
		if err != nil {
			return err
		}
		printSandboxExecutionCandidate(a, result)
		return nil
	case "candidates":
		fs := newFlagSet("run sandbox candidates", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox execution candidates")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox candidates <run-id> [--limit <n>]")
		}
		values, err := service.ListExecutionCandidates(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox execution candidates")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tpreparation=%s\tapproval=%s\tbackend_enabled=false\texecution_authorized=false\tvalidated_at=%s\n",
				value.Candidate.ID, value.Candidate.PreparationID, value.Candidate.ApprovalStatus,
				value.Candidate.ValidatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "candidate-show":
		fs := newFlagSet("run sandbox candidate-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox candidate-show <candidate-id>")
		}
		value, err := service.GetExecutionCandidate(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxExecutionCandidate(a, value)
		return nil
	case "begin":
		fs := newFlagSet("run sandbox begin", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable lifecycle creation operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox begin <candidate-id> --manifest <manifest.json> --operation-key <key> [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		value, err := service.BeginDisabledExecution(ctx, application.BeginSandboxExecutionRequest{
			CandidateID: fs.Arg(0), Manifest: manifest,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	case "preflight":
		fs := newFlagSet("run sandbox preflight", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable preflight operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox preflight <execution-id> --manifest <manifest.json> --operation-key <key> [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		value, err := service.PrepareDisabledPreflight(ctx,
			application.PrepareSandboxPreflightRequest{
				ExecutionID: fs.Arg(0), Manifest: manifest,
				OperationKey: *operationKey, RequestedBy: *operator,
			})
		if err != nil {
			return err
		}
		printSandboxPreflight(a, value)
		return nil
	case "preflights":
		fs := newFlagSet("run sandbox preflights", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox preflights")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox preflights <run-id> [--limit <n>]")
		}
		values, err := service.ListDisabledPreflights(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox preflights")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\texecution=%s\tstatus=%s\trequired_checks=%d\tverified_checks=0\toutput_slots=%d\tbackend_enabled=false\texecution_authorized=false\tcreated_at=%s\n",
				value.ID, value.ExecutionID, value.Status, len(value.Handshake.Checks),
				value.OutputPlan.SlotCount, value.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "preflight-show":
		fs := newFlagSet("run sandbox preflight-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox preflight-show <preflight-id>")
		}
		value, err := service.GetDisabledPreflight(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxPreflight(a, value)
		return nil
	case "evidence":
		fs := newFlagSet("run sandbox evidence", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied Docker sandbox manifest JSON file")
		imageDigest := fs.String("image-digest", "", "exact OCI sha256 image digest")
		operationKey := fs.String("operation-key", "", "stable backend evidence operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "image-digest": true, "operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*imageDigest) == "" || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox evidence <preflight-id> --manifest <manifest.json> --image-digest <sha256:digest> --operation-key <key> [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		value, err := service.RecordSimulatedBackendEvidence(ctx,
			application.RecordSandboxBackendEvidenceRequest{
				PreflightID: fs.Arg(0), Manifest: manifest, ImageDigest: *imageDigest,
				OperationKey: *operationKey, RequestedBy: *operator,
			})
		if err != nil {
			return err
		}
		printSandboxBackendEvidence(a, value)
		return nil
	case "evidences":
		fs := newFlagSet("run sandbox evidences", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox backend evidence records")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox evidences <run-id> [--limit <n>]")
		}
		values, err := service.ListBackendEvidence(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox backend evidence")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tpreflight=%s\tstatus=%s\tsimulated_satisfied=%d\tproduction_verified=0\tbackend_enabled=false\texecution_authorized=false\tcreated_at=%s\n",
				value.ID, value.PreflightID, value.Report.Status, len(value.Report.Items),
				value.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "evidence-show":
		fs := newFlagSet("run sandbox evidence-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox evidence-show <evidence-id>")
		}
		value, err := service.GetBackendEvidence(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxBackendEvidence(a, value)
		return nil
	case "output-simulate":
		fs := newFlagSet("run sandbox output-simulate", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied Docker sandbox manifest JSON file")
		fixturePath := fs.String("fixture", "", "bounded output fixture JSON file")
		operationKey := fs.String("operation-key", "", "stable output simulation operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "fixture": true, "operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*fixturePath) == "" || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox output-simulate <evidence-id> --manifest <manifest.json> --fixture <fixture.json> --operation-key <key> [--operator <id>]")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		fixture, err := readSandboxOutputFixture(*fixturePath)
		if err != nil {
			return err
		}
		value, err := service.SimulateOutputTransaction(ctx,
			application.SimulateSandboxOutputRequest{
				EvidenceID: fs.Arg(0), Manifest: manifest, Fixture: fixture,
				OperationKey: *operationKey, RequestedBy: *operator,
			})
		if err != nil {
			return err
		}
		printSandboxOutputSimulation(a, value)
		return nil
	case "output-simulations":
		fs := newFlagSet("run sandbox output-simulations", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox output simulations")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox output-simulations <run-id> [--limit <n>]")
		}
		values, err := service.ListOutputSimulations(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox output simulations")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tevidence=%s\tstatus=%s\tstaged_outputs=%d\tfake_artifacts=%d\tproduction_artifacts=0\tartifact_commit_authorized=false\tcreated_at=%s\n",
				value.ID, value.EvidenceID, value.Status, value.StagedOutputCount,
				value.FakeArtifactCount, value.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "output-simulation-show":
		fs := newFlagSet("run sandbox output-simulation-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox output-simulation-show <simulation-id>")
		}
		value, err := service.GetOutputSimulation(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxOutputSimulation(a, value)
		return nil
	case "observe":
		fs := newFlagSet("run sandbox observe", a.errOut)
		simulationID := fs.String("simulation", "", "exact v52 output simulation identity")
		manifestPath := fs.String("manifest", "", "resupplied Docker sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable Docker observation operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		confirmed := fs.Bool("confirm-readonly-probe", false,
			"confirm fixed-endpoint read-only Docker Engine API observation")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"simulation": true, "manifest": true, "operation-key": true,
			"operator": true, "confirm-readonly-probe": false,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*simulationID) == "" ||
			strings.TrimSpace(*manifestPath) == "" || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox observe <evidence-id> --simulation <simulation-id> --manifest <manifest.json> --operation-key <key> --confirm-readonly-probe [--operator <id>]")
		}
		if !*confirmed {
			return apperror.New(apperror.CodeFailedPrecondition,
				"read-only Docker observation requires --confirm-readonly-probe")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		value, err := service.ObserveDockerBackend(ctx, application.ObserveDockerBackendRequest{
			EvidenceID: fs.Arg(0), OutputSimulationID: *simulationID, Manifest: manifest,
			OperationKey: *operationKey, RequestedBy: *operator,
		})
		if err != nil {
			return err
		}
		printSandboxDockerObservation(a, value)
		return nil
	case "observations":
		fs := newFlagSet("run sandbox observations", a.errOut)
		limit := fs.Int("limit", 100, "maximum read-only Docker observations")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox observations <run-id> [--limit <n>]")
		}
		values, err := service.ListDockerObservations(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no read-only Docker observations")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tevidence=%s\tsimulation=%s\tstatus=%s\tfailure=%s\tobserved=%t\tproduction_verified=false\tbackend_enabled=false\texecution_authorized=false\tcreated_at=%s\n",
				value.ID, value.EvidenceID, value.OutputSimulationID, value.Report.Status,
				value.Report.FailureCode, value.Report.ProductionObserved,
				value.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "observation-show":
		fs := newFlagSet("run sandbox observation-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox observation-show <observation-id>")
		}
		value, err := service.GetDockerObservation(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxDockerObservation(a, value)
		return nil
	case "docker-plan":
		fs := newFlagSet("run sandbox docker-plan", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied Docker sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable Docker container plan operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		confirmed := fs.Bool("confirm-fake-write", false,
			"confirm an in-memory fake Docker write transaction with zero daemon writes")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "operator": true,
			"confirm-fake-write": false,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox docker-plan <observation-id> --manifest <manifest.json> --operation-key <key> --confirm-fake-write [--operator <id>]")
		}
		if !*confirmed {
			return apperror.New(apperror.CodeFailedPrecondition,
				"Docker container planning requires --confirm-fake-write")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		value, err := service.CompileDockerContainerPlan(ctx,
			application.CompileDockerContainerPlanRequest{ObservationID: fs.Arg(0),
				Manifest: manifest, OperationKey: *operationKey, RequestedBy: *operator})
		if err != nil {
			return err
		}
		printSandboxDockerContainerPlan(a, value)
		return nil
	case "docker-plans":
		fs := newFlagSet("run sandbox docker-plans", a.errOut)
		limit := fs.Int("limit", 100, "maximum Docker container plans")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox docker-plans <run-id> [--limit <n>]")
		}
		values, err := service.ListDockerContainerPlans(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no Docker container plans")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tobservation=%s\tstatus=%s\tcontrols=%d\tfake_write_steps=%d\tdaemon_writes=0\tproduction_submitted=false\texecution_authorized=false\tcreated_at=%s\n",
				value.ID, value.ObservationID, value.Status, len(value.Controls),
				value.Transaction.CommittedStepCount,
				value.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "docker-plan-show":
		fs := newFlagSet("run sandbox docker-plan-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox docker-plan-show <plan-id>")
		}
		value, err := service.GetDockerContainerPlan(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxDockerContainerPlan(a, value)
		return nil
	case "docker-rehearse":
		fs := newFlagSet("run sandbox docker-rehearse", a.errOut)
		manifestPath := fs.String("manifest", "", "resupplied Docker sandbox manifest JSON file")
		operationKey := fs.String("operation-key", "", "stable Docker rehearsal operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		confirmed := fs.Bool("confirm-daemon-write", false,
			"confirm bounded create-inspect-remove writes to the fixed local Docker socket")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"manifest": true, "operation-key": true, "operator": true,
			"confirm-daemon-write": false,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*manifestPath) == "" ||
			strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox docker-rehearse <plan-id> --manifest <manifest.json> --operation-key <key> --confirm-daemon-write [--operator <id>]")
		}
		if !*confirmed {
			return apperror.New(apperror.CodeFailedPrecondition,
				"Docker container rehearsal requires --confirm-daemon-write")
		}
		manifest, err := readSandboxManifest(*manifestPath)
		if err != nil {
			return err
		}
		if a.dockerWriteTransport == nil {
			service.WithDockerContainerWriteTransport(
				sandbox.NewLocalDockerContainerWriteTransport())
		}
		value, err := service.RehearseDockerContainer(ctx,
			application.RehearseDockerContainerRequest{PlanID: fs.Arg(0),
				Manifest: manifest, OperationKey: *operationKey, RequestedBy: *operator,
				OperatorConfirmed: true})
		if err != nil {
			return err
		}
		printSandboxDockerContainerRehearsal(a, value)
		return nil
	case "docker-rehearsals":
		fs := newFlagSet("run sandbox docker-rehearsals", a.errOut)
		limit := fs.Int("limit", 100, "maximum Docker container rehearsals")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox docker-rehearsals <run-id> [--limit <n>]")
		}
		values, err := service.ListDockerContainerRehearsals(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no Docker container rehearsals")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tplan=%s\tstatus=%s\tdaemon_writes=%d\tcontainer_started=false\tprocess_executed=false\tproduction_verified=false\texecution_authorized=false\tcreated_at=%s\n",
				value.ID, value.PlanID, value.Status, value.DaemonWriteCount,
				value.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "docker-rehearsal-show":
		fs := newFlagSet("run sandbox docker-rehearsal-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox docker-rehearsal-show <rehearsal-id>")
		}
		value, err := service.GetDockerContainerRehearsal(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxDockerContainerRehearsal(a, value)
		return nil
	case "cancel":
		fs := newFlagSet("run sandbox cancel", a.errOut)
		operationKey := fs.String("operation-key", "", "stable cancellation operation key")
		operator := fs.String("operator", "cli_operator", "operator identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox cancel <execution-id> --operation-key <key> [--operator <id>]")
		}
		value, err := service.CancelDisabledExecution(ctx, application.CancelSandboxExecutionRequest{
			ExecutionID: fs.Arg(0), OperationKey: *operationKey, RequestedBy: *operator,
		})
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	case "cleanup":
		fs := newFlagSet("run sandbox cleanup", a.errOut)
		operationKey := fs.String("operation-key", "", "stable cleanup operation key")
		operator := fs.String("operator", "cli_operator", "reconciler identity")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{
			"operation-key": true, "operator": true,
		})); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*operationKey) == "" {
			return errors.New("usage: cyberagent run sandbox cleanup <execution-id> --operation-key <key> [--operator <id>]")
		}
		value, err := service.CleanupDisabledExecution(ctx, application.CleanupSandboxExecutionRequest{
			ExecutionID: fs.Arg(0), OperationKey: *operationKey, ReconciledBy: *operator,
		})
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	case "executions":
		fs := newFlagSet("run sandbox executions", a.errOut)
		limit := fs.Int("limit", 100, "maximum sandbox executions")
		if err := fs.Parse(reorderFlags(args[1:], map[string]bool{"limit": true})); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox executions <run-id> [--limit <n>]")
		}
		values, err := service.ListDisabledExecutions(ctx, fs.Arg(0), *limit)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			fmt.Fprintln(a.out, "no sandbox executions")
			return nil
		}
		for _, value := range values {
			fmt.Fprintf(a.out, "%s\tcandidate=%s\tstatus=%s\tlease_generation=%d\tlease_status=%s\tbackend_enabled=false\texecution_authorized=false\tbackend_started=false\tcreated_at=%s\n",
				value.Execution.ID, value.Execution.CandidateID, value.Status,
				value.Lease.Generation, value.Lease.Status,
				value.Execution.CreatedAt.Format(timeFormatRFC3339Nano))
		}
		return nil
	case "execution-show":
		fs := newFlagSet("run sandbox execution-show", a.errOut)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: cyberagent run sandbox execution-show <execution-id>")
		}
		value, err := service.GetDisabledExecution(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		printSandboxLifecycle(a, value)
		return nil
	default:
		return fmt.Errorf("unknown run sandbox subcommand %q", args[0])
	}
}

func printSandboxPreflight(a *App, value sandbox.DisabledPreflight) {
	fmt.Fprintf(a.out, "preflight: %s\nexecution: %s\ncandidate: %s\npreparation: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nbackend: %s\nstatus: %s\nmanifest_fingerprint: %s\nauthorization_fingerprint: %s\npolicy_fingerprint: %s\nmount_binding_fingerprint: %s\ninput_artifact_digest: %s\nhandshake_protocol: %s\ninspector: %s\nbackend_available: false\nrequired_checks: %d\nverified_checks: 0\ncontainer_identity_bound: false\noutput_protocol: %s\noutput_slots: %d\nmax_output_bytes: %d\npartial_failure_policy: %s\ntruncation_policy: %s\nmime_policy: %s\nfile_type_policy: %s\nrestart_policy: %s\nraw_paths_stored: false\noutput_export_enabled: false\nartifact_commit_authorized: false\nbackend_enabled: false\nexecution_authorized: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		value.ID, value.ExecutionID, value.CandidateID, value.PreparationID, value.RunID,
		value.MissionID, value.WorkspaceID, value.ProtocolVersion, value.Backend, value.Status,
		value.ManifestFingerprint, value.AuthorizationFingerprint, value.PolicyFingerprint,
		value.MountBindingFingerprint, value.InputArtifactDigest,
		value.Handshake.ProtocolVersion, value.Handshake.InspectorName,
		len(value.Handshake.Checks), value.OutputPlan.ProtocolVersion,
		value.OutputPlan.SlotCount, value.OutputPlan.MaxOutputBytes,
		value.OutputPlan.PartialFailurePolicy, value.OutputPlan.TruncationPolicy,
		value.OutputPlan.MIMEPolicy, value.OutputPlan.FileTypePolicy,
		value.OutputPlan.RestartPolicy, value.RequestedBy,
		value.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	fmt.Fprintln(a.out, "checks:")
	for _, check := range value.Handshake.Checks {
		fmt.Fprintf(a.out, "%d\t%s\trequired=%t\tverified=%t\tevidence=%s\n",
			check.Ordinal, check.Name, check.Required, check.Verified, check.EvidenceState)
	}
	fmt.Fprintln(a.out, "outputs:")
	for _, slot := range value.OutputPlan.Slots {
		fmt.Fprintf(a.out, "%d\tkind=%s\tregular_file_required=%t\tsymlink_rejected=%t\tspecial_file_rejected=%t\tmime_detection_required=%t\tredaction_required=%t\tartifact_commit_authorized=false\n",
			slot.Ordinal, slot.Kind, slot.RegularFileRequired, slot.SymlinkRejected,
			slot.SpecialFileRejected, slot.MIMEDetectionRequired, slot.RedactionRequired)
	}
}

func printSandboxBackendEvidence(a *App, value sandbox.BackendEvidence) {
	report := value.Report
	fmt.Fprintf(a.out, "evidence: %s\npreflight: %s\nexecution: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nsource: %s\ntrust_class: %s\nstatus: %s\nbackend: %s\nimage_digest: %s\nmanifest_fingerprint: %s\nthreat_model_fingerprint: %s\ndaemon_capabilities_fingerprint: %s\nmount_plan_fingerprint: %s\nnetwork_plan_fingerprint: %s\nsecret_plan_fingerprint: %s\ncontainer_config_fingerprint: %s\nresource_plan_fingerprint: %s\ntermination_plan_fingerprint: %s\norphan_plan_fingerprint: %s\noutput_plan_fingerprint: %s\nsimulated_satisfied: %d\nproduction_verified: 0\nverified_checks: 0\nbackend_available: false\nbackend_enabled: false\nexecution_authorized: false\nartifact_commit_authorized: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		value.ID, value.PreflightID, value.ExecutionID, value.RunID, value.MissionID,
		value.WorkspaceID, report.ProtocolVersion, report.Source, report.TrustClass,
		report.Status, report.Backend, report.ImageDigest, value.ManifestFingerprint,
		value.ThreatModelFingerprint, report.DaemonCapabilitiesFingerprint,
		report.MountPlanFingerprint, report.NetworkPlanFingerprint,
		report.SecretPlanFingerprint, report.ContainerConfigFingerprint,
		report.ResourcePlanFingerprint, report.TerminationPlanFingerprint,
		report.OrphanPlanFingerprint, report.OutputPlanFingerprint, len(report.Items),
		value.RequestedBy, value.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	fmt.Fprintln(a.out, "items:")
	for _, item := range report.Items {
		fmt.Fprintf(a.out, "%d\t%s\tstate=%s\tsatisfied=%t\tverified=false\tevidence_digest=%s\n",
			item.Ordinal, item.Name, item.EvidenceState, item.Satisfied, item.EvidenceDigest)
	}
}

func printSandboxOutputSimulation(a *App, value sandbox.OutputSimulation) {
	fmt.Fprintf(a.out, "simulation: %s\nevidence: %s\npreflight: %s\nexecution: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nstatus: %s\noutput_plan_fingerprint: %s\nexpected_slots: %d\nstaged_outputs: %d\nstaged_output_bytes: %d\nfake_artifacts: %d\nproduction_artifacts: 0\nall_or_nothing: true\nsimulation_only: true\nartifact_commit_authorized: false\nbackend_enabled: false\nexecution_authorized: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		value.ID, value.EvidenceID, value.PreflightID, value.ExecutionID, value.RunID,
		value.MissionID, value.WorkspaceID, value.ProtocolVersion, value.Status,
		value.OutputPlanFingerprint, value.ExpectedSlotCount, value.StagedOutputCount,
		value.StagedOutputBytes, value.FakeArtifactCount, value.RequestedBy,
		value.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	fmt.Fprintln(a.out, "outputs:")
	for _, descriptor := range value.Descriptors {
		fmt.Fprintf(a.out, "%d\tkind=%s\tmime=%s\tsha256=%s\tsize_bytes=%d\tredacted=%t\n",
			descriptor.Ordinal, descriptor.Kind, descriptor.MIME, descriptor.SHA256,
			descriptor.SizeBytes, descriptor.Redacted)
	}
}

func printSandboxDockerObservation(a *App, value sandbox.DockerObservation) {
	report := value.Report
	observed := 0
	for _, item := range report.Items {
		if item.Observed {
			observed++
		}
	}
	fmt.Fprintf(a.out, "observation: %s\nevidence: %s\noutput_simulation: %s\npreflight: %s\nexecution: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nsource: %s\ntrust_class: %s\nstatus: %s\nendpoint_class: %s\nendpoint_fingerprint: %s\nbinding_fingerprint: %s\nimage_digest: %s\nfailure_code: %s\ndaemon_reachable: %t\nimage_inspected: %t\nobservation_complete: %t\nproduction_observed: %t\nproduction_verified: false\nobserved_items: %d\nverified_items: 0\napi_version: %s\nmin_api_version: %s\nengine_version: %s\nos_type: %s\narchitecture: %s\nrootless: %t\nuser_namespace_enabled: %t\nprivate_mount_state: %s\ncgroup_version: %s\nncpu: %d\nmemory_bytes: %d\npids_limit_supported: %t\nimage_os_type: %s\nimage_architecture: %s\nimage_size_bytes: %d\nimage_user_state: %s\ndaemon_identity_fingerprint: %s\ncapability_fingerprint: %s\nimage_fingerprint: %s\nobservation_fingerprint: %s\nbackend_available: false\nbackend_enabled: false\nexecution_authorized: false\nartifact_commit_authorized: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		value.ID, value.EvidenceID, value.OutputSimulationID, value.PreflightID,
		value.ExecutionID, value.RunID, value.MissionID, value.WorkspaceID,
		report.ProtocolVersion, report.Source, report.TrustClass, report.Status,
		report.EndpointClass, report.EndpointFingerprint, report.BindingFingerprint,
		report.ImageDigest, report.FailureCode, report.DaemonReachable,
		report.ImageInspected, report.ObservationComplete, report.ProductionObserved,
		observed, report.APIVersion, report.MinAPIVersion, report.EngineVersion,
		report.OSType, report.Architecture, report.Rootless,
		report.UserNamespaceEnabled, report.PrivateMountState, report.CgroupVersion,
		report.NCPU, report.MemoryBytes, report.PidsLimitSupported, report.ImageOSType,
		report.ImageArchitecture, report.ImageSizeBytes, report.ImageUserState,
		report.DaemonIdentityFingerprint, report.CapabilityFingerprint,
		report.ImageFingerprint, report.ObservationFingerprint, value.RequestedBy,
		value.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	fmt.Fprintln(a.out, "items:")
	for _, item := range report.Items {
		fmt.Fprintf(a.out, "%d\t%s\tstate=%s\tobserved=%t\tverified=false\tevidence_digest=%s\n",
			item.Ordinal, item.Name, item.State, item.Observed, item.EvidenceDigest)
	}
}

func printSandboxDockerContainerPlan(a *App, value sandbox.DockerContainerPlan) {
	fmt.Fprintf(a.out, "docker_plan: %s\nobservation: %s\nevidence: %s\noutput_simulation: %s\npreflight: %s\nexecution: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nsource: %s\ntrust_class: %s\nstatus: %s\nmanifest_fingerprint: %s\nauthority_fingerprint: %s\nspec_fingerprint: %s\nplan_fingerprint: %s\nos_type: %s\narchitecture: %s\ncontainer_user: %s\nread_only_rootfs: %t\nno_new_privileges: %t\ndrop_all_capabilities: %t\ninit_enabled: %t\nmounts: %d\nread_only_mounts: %d\nwritable_mounts: %d\ndedicated_output_mounts: %d\nprivate_propagation_mounts: %d\nenvironment_bindings: %d\nsecret_references: %d\nsecrets_ephemeral: %t\nsecrets_metadata_excluded: %t\ninput_artifacts: %d\noutputs: %d\nnetwork_mode: %s\nnetwork_targets: %d\nnetwork_default_deny: %t\nexact_network_allowlist: %t\nnetwork_guard_required: %t\nnano_cpus: %d\nmemory_bytes: %d\npids: %d\nmax_output_bytes: %d\ntimeout_seconds: %d\ngrace_period_millis: %d\nreconcile_before_create: %t\nremove_on_rollback: %t\nexport_after_stop: %t\nremove_after_export: %t\ntransaction_protocol: %s\ntransaction_status: %s\nfake_write_steps: %d\ndaemon_writes: 0\nbackend_touched: false\nsimulation_only: true\nproduction_submitted: false\nproduction_verified: false\nbackend_available: false\nbackend_enabled: false\nexecution_authorized: false\nartifact_commit_authorized: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		value.ID, value.ObservationID, value.EvidenceID, value.OutputSimulationID,
		value.PreflightID, value.ExecutionID, value.RunID, value.MissionID,
		value.WorkspaceID, value.ProtocolVersion, value.Source, value.TrustClass,
		value.Status, value.ManifestFingerprint, value.AuthorityFingerprint,
		value.SpecFingerprint, value.PlanFingerprint, value.OSType, value.Architecture,
		value.ContainerUser, value.ReadOnlyRootFS, value.NoNewPrivileges,
		value.DropAllCapabilities, value.InitEnabled, value.MountCount,
		value.ReadOnlyMountCount, value.WritableMountCount, value.DedicatedOutputMounts,
		value.PrivatePropagationMounts, value.EnvironmentCount,
		value.SecretReferenceCount, value.SecretsEphemeral,
		value.SecretsMetadataExcluded, value.InputArtifactCount, value.OutputCount,
		value.NetworkMode, value.NetworkTargetCount, value.NetworkDefaultDeny,
		value.ExactNetworkAllowlist, value.NetworkGuardRequired, value.NanoCPUs,
		value.MemoryBytes, value.PIDs, value.MaxOutputBytes, value.TimeoutSeconds,
		value.GracePeriodMillis, value.ReconcileBeforeCreate, value.RemoveOnRollback,
		value.ExportAfterStop, value.RemoveAfterExport, value.Transaction.ProtocolVersion,
		value.Transaction.Status, value.Transaction.CommittedStepCount,
		value.RequestedBy, value.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	fmt.Fprintln(a.out, "controls:")
	for _, control := range value.Controls {
		fmt.Fprintf(a.out, "%d\t%s\tstate=%s\tplanned=true\tapplied=false\tverified=false\n",
			control.Ordinal, control.Name, control.State)
	}
	fmt.Fprintln(a.out, "fake_write_transaction:")
	for _, step := range value.Transaction.Steps {
		fmt.Fprintf(a.out, "%d\t%s\tstate=%s\tsimulated=true\tproduction_applied=false\n",
			step.Ordinal, step.Name, step.State)
	}
}

func printSandboxDockerContainerRehearsal(a *App, value sandbox.DockerContainerRehearsal) {
	fmt.Fprintf(a.out, "docker_rehearsal: %s\ndocker_plan: %s\nobservation: %s\nevidence: %s\noutput_simulation: %s\npreflight: %s\nexecution: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\ntransport_protocol: %s\nsource: %s\ntrust_class: %s\nstatus: %s\nendpoint_class: %s\nmanifest_fingerprint: %s\nauthority_fingerprint: %s\nspec_fingerprint: %s\nplan_fingerprint: %s\nrequest_fingerprint: %s\ntransport_fingerprint: %s\nrehearsal_fingerprint: %s\nnetwork_mode: %s\nenvironment_bindings: %d\nsecret_references: %d\nconfiguration_matched: %t\nreconciled_containers: %d\ndaemon_reads: %d\ndaemon_writes: %d\ncontainer_created: %t\ncontainer_inspected: %t\ncontainer_removed: %t\ncontainer_started: false\nprocess_executed: false\nimage_pulled: false\noutput_exported: false\ncleanup_confirmed: %t\ndaemon_reachable: %t\ndaemon_write_submitted: %t\nproduction_execution_submitted: false\nproduction_verified: false\nbackend_enabled: false\nexecution_authorized: false\nartifact_commit_authorized: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		value.ID, value.PlanID, value.ObservationID, value.EvidenceID,
		value.OutputSimulationID, value.PreflightID, value.ExecutionID, value.RunID,
		value.MissionID, value.WorkspaceID, value.ProtocolVersion,
		value.Result.ProtocolVersion, value.Source, value.TrustClass, value.Status,
		value.EndpointClass, value.ManifestFingerprint, value.AuthorityFingerprint,
		value.SpecFingerprint, value.PlanFingerprint, value.RequestFingerprint,
		value.TransportFingerprint, value.RehearsalFingerprint, value.NetworkMode,
		value.EnvironmentCount, value.SecretReferenceCount, value.ConfigurationMatched,
		value.ReconciledContainerCount, value.DaemonReadCount, value.DaemonWriteCount,
		value.Result.ContainerCreated, value.Result.ContainerInspected,
		value.Result.ContainerRemoved, value.CleanupConfirmed, value.DaemonReachable,
		value.DaemonWriteSubmitted, value.RequestedBy,
		value.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	fmt.Fprintln(a.out, "write_transport_steps:")
	for _, step := range value.Result.Steps {
		fmt.Fprintf(a.out, "%d\t%s\tstate=%s\tdaemon_reads=%d\tdaemon_writes=%d\tproduction_applied=%t\n",
			step.Ordinal, step.Name, step.State, step.DaemonReads, step.DaemonWrites,
			step.ProductionApplied)
	}
}

func printSandboxLifecycle(a *App, value sandbox.Lifecycle) {
	execution := value.Execution
	fmt.Fprintf(a.out, "execution: %s\ncandidate: %s\npreparation: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nstatus: %s\nmanifest_fingerprint: %s\nauthorization_fingerprint: %s\npolicy_fingerprint: %s\nmount_binding_fingerprint: %s\ninput_artifacts: %d\ninput_artifact_bytes: %d\ninput_artifact_digest: %s\ncapture_stdout: %t\ncapture_stderr: %t\noutput_paths: %d\nmax_output_bytes: %d\noutput_plan_fingerprint: %s\nlease_generation: %d\nlease_status: %s\ncancellation_requested: %t\ncleanup_complete: %t\nbackend_enabled: false\nexecution_authorized: false\nbackend_started: false\nrequested_by: %s\ncreated_at: %s\nreplayed: %t\n",
		execution.ID, execution.CandidateID, execution.PreparationID, execution.RunID,
		execution.MissionID, execution.WorkspaceID, execution.ProtocolVersion, value.Status,
		execution.ManifestFingerprint, execution.AuthorizationFingerprint,
		execution.PolicyFingerprint, execution.MountBindingFingerprint,
		execution.InputArtifactCount, execution.InputArtifactBytes,
		execution.InputArtifactDigest, execution.OutputPlan.CaptureStdout,
		execution.OutputPlan.CaptureStderr, execution.OutputPlan.OutputPathCount,
		execution.OutputPlan.MaxOutputBytes, execution.OutputPlan.Fingerprint,
		value.Lease.Generation, value.Lease.Status, value.Cancellation != nil,
		value.Cleanup != nil, execution.RequestedBy,
		execution.CreatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
	if value.Cleanup != nil {
		fmt.Fprintf(a.out, "cleanup_protocol: %s\ncleanup_outcome: %s\ncleanup_lease_generation: %d\ncancellation_observed: %t\ninput_artifacts_verified: %t\noutput_artifacts: %d\norphan_detected: false\norphan_reaped: false\ncleaned_at: %s\n",
			value.Cleanup.ProtocolVersion, value.Cleanup.Outcome,
			value.Cleanup.LeaseGeneration, value.Cleanup.CancellationObserved,
			value.Cleanup.InputArtifactsVerified, value.Cleanup.OutputArtifactCount,
			value.Cleanup.CompletedAt.Format(timeFormatRFC3339Nano))
	}
}

func printSandboxExecutionCandidate(a *App, value sandbox.ValidatedExecutionCandidate) {
	candidate := value.Candidate
	fmt.Fprintf(a.out, "candidate: %s\npreparation: %s\nrun: %s\nmission: %s\nworkspace: %s\nprotocol: %s\nmanifest_fingerprint: %s\nauthorization_fingerprint: %s\nmount_binding_fingerprint: %s\napproval: %s\napproval_status: %s\nmounts: %d\nregular_file_mounts: %d\ndirectory_mounts: %d\ntokens_used: %d\nexecution_millis_used: %d\ntool_calls_used: %d\nbudget_checked: true\nlease_quiescent: true\nbackend_enabled: false\nexecution_authorized: false\nrequested_by: %s\nvalidated_at: %s\nreplayed: %t\n",
		candidate.ID, candidate.PreparationID, candidate.RunID, candidate.MissionID,
		candidate.WorkspaceID, candidate.ProtocolVersion, candidate.ManifestFingerprint,
		candidate.AuthorizationFingerprint, candidate.MountBindingFingerprint,
		candidate.ApprovalID, candidate.ApprovalStatus, candidate.MountCount,
		candidate.RegularFileMountCount, candidate.DirectoryMountCount,
		candidate.TokensUsed, candidate.ExecutionMillisUsed, candidate.ToolCallsUsed,
		candidate.RequestedBy, candidate.ValidatedAt.Format(timeFormatRFC3339Nano), value.Replayed)
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func printSandboxManifestIntent(a *App, value sandbox.PreparedIntent) {
	preparation := value.Preparation
	validation := value.Validation
	fmt.Fprintf(a.out, "preparation: %s\nrun: %s\nmission: %s\nworkspace: %s\ncancellation: %s\nprotocol: %s\nbackend: %s\nmanifest_fingerprint: %s\nauthorization_fingerprint: %s\narguments: %d\nmounts: %d\nwritable_mounts: %d\nenvironment_bindings: %d\nsecret_references: %d\nnetwork_mode: %s\nallowed_targets: %d\ninput_artifacts: %d\noutputs: %d\ntimeout_seconds: %d\npolicy_allowed: %t\nneeds_approval: %t\nrisk: %s\napproval_status: %s\nvalidator: %s\nbackend_enabled: false\nexecution_authorized: false\nrequested_by: %s\nreplayed: %t\n",
		preparation.ID, preparation.RunID, preparation.MissionID, preparation.WorkspaceID,
		preparation.CancellationID, preparation.ProtocolVersion, preparation.Backend,
		preparation.ManifestFingerprint, preparation.AuthorizationFingerprint,
		preparation.CommandArgumentCount, preparation.MountCount, preparation.WritableMountCount,
		preparation.EnvironmentCount,
		preparation.SecretReferenceCount, preparation.NetworkMode,
		preparation.AllowedTargetCount, preparation.InputArtifactCount,
		preparation.OutputCount, preparation.TimeoutSeconds,
		validation.PolicyAllowed, validation.NeedsApproval, validation.Risk,
		validation.ApprovalStatus, validation.ValidatorName, preparation.RequestedBy,
		value.Replayed)
}

func readSandboxManifest(path string) (sandbox.Manifest, error) {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return sandbox.Manifest{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, sandbox.MaxManifestBytes+1))
	if err != nil {
		return sandbox.Manifest{}, err
	}
	manifest, err := sandbox.DecodeManifest(data)
	if err != nil {
		return sandbox.Manifest{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest is invalid: "+err.Error(), err)
	}
	return manifest, nil
}

func readSandboxOutputFixture(path string) (sandbox.OutputFixture, error) {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return sandbox.OutputFixture{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, sandbox.MaxOutputFixtureBytes+1))
	if err != nil {
		return sandbox.OutputFixture{}, err
	}
	fixture, err := sandbox.DecodeOutputFixture(data)
	if err != nil {
		return sandbox.OutputFixture{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox output fixture is invalid: "+err.Error(), err)
	}
	return fixture, nil
}

func defaultSandboxManifestTemplate() sandbox.Manifest {
	return sandbox.Manifest{
		ProtocolVersion: sandbox.ManifestProtocolVersion,
		Backend:         sandbox.BackendNoop,
		Command: sandbox.CommandSpec{
			Executable: "go", Arguments: []string{"test", "./..."},
			WorkingDirectory: "/workspace",
		},
		Mounts: []sandbox.Mount{{
			Source: ".", Target: "/workspace", Access: sandbox.MountReadOnly,
		}},
		Network: sandbox.NetworkScope{Mode: "disabled"},
		Resources: sandbox.ResourceLimits{
			CPUQuotaMillis: 1000, MemoryBytes: 256 * 1024 * 1024,
			PIDs: 64, MaxOutputBytes: 4 * 1024 * 1024,
		},
		Output:         sandbox.OutputSpec{CaptureStdout: true, CaptureStderr: true},
		TimeoutSeconds: 300,
		Cancellation:   sandbox.CancellationSpec{GracePeriodMillis: 2000},
	}
}
