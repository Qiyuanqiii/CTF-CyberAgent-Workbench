package sandbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestManifestNormalizationAndFingerprintAreDeterministic(t *testing.T) {
	left := validManifest()
	left.Network = NetworkScope{Mode: "allowlist", AllowedTargets: []string{
		"EXAMPLE.com.", "127.0.0.1",
	}}
	left.Environment = []EnvironmentBinding{
		{Name: "Z_MODE", Source: EnvironmentLiteral, Value: "test"},
		{Name: "API_TOKEN", Source: EnvironmentSecretRef, Value: "secret-store/api-token"},
	}
	left.InputArtifactIDs = []string{"artifact-z", "artifact-a"}
	left.Mounts = append(left.Mounts, Mount{Source: "src", Target: "/src", Access: MountReadOnly})
	right := left
	right.Network.AllowedTargets = []string{"127.0.0.1", "example.com"}
	right.Environment = []EnvironmentBinding{left.Environment[1], left.Environment[0]}
	right.InputArtifactIDs = []string{"artifact-a", "artifact-z"}
	right.Mounts = []Mount{left.Mounts[1], left.Mounts[0]}

	normalized, err := NormalizeManifest(left)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Network.AllowedTargets[0] != "127.0.0.1" ||
		normalized.Network.AllowedTargets[1] != "example.com" ||
		normalized.Environment[0].Name != "API_TOKEN" ||
		normalized.InputArtifactIDs[0] != "artifact-a" {
		t.Fatalf("manifest sets were not normalized: %#v", normalized)
	}
	leftFingerprint, err := left.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	rightFingerprint, err := right.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if leftFingerprint != rightFingerprint || len(leftFingerprint) != 64 {
		t.Fatalf("semantic manifest fingerprints differ: left=%s right=%s", leftFingerprint, rightFingerprint)
	}
}

func TestDecodeManifestRejectsAmbiguousJSON(t *testing.T) {
	manifest := validManifest()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeManifest(encoded); err != nil {
		t.Fatalf("valid manifest was rejected: %v", err)
	}
	cases := map[string]string{
		"unknown field": strings.TrimSuffix(string(encoded), "}") + `,"execute":true}`,
		"case-folded field": strings.Replace(string(encoded), `"protocol_version"`,
			`"Protocol_Version"`, 1),
		"duplicate nested field": strings.Replace(string(encoded),
			`"executable":"go"`, `"executable":"go","executable":"powershell"`, 1),
		"missing cancellation": strings.Replace(string(encoded),
			`,"cancellation":{"grace_period_millis":2000}`, "", 1),
		"excessive JSON depth": strings.Replace(string(encoded), `"backend":"noop"`,
			`"backend":`+strings.Repeat("[", maxManifestJSONDepth+1)+`"noop"`+
				strings.Repeat("]", maxManifestJSONDepth+1), 1),
		"trailing JSON": string(encoded) + `{}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeManifest([]byte(raw)); err == nil {
				t.Fatalf("ambiguous manifest was accepted: %s", raw)
			}
		})
	}
	if _, err := DecodeManifest([]byte(strings.Repeat(" ", MaxManifestBytes+1))); err == nil {
		t.Fatal("oversized manifest was accepted")
	}
}

func TestManifestRejectsCapabilityAndPathAmbiguity(t *testing.T) {
	cases := map[string]func(*Manifest){
		"secret literal": func(value *Manifest) {
			value.Environment = []EnvironmentBinding{{
				Name: "API_TOKEN", Source: EnvironmentLiteral, Value: "do-not-accept",
			}}
		},
		"credential in command argument": func(value *Manifest) {
			value.Command.Arguments = []string{"sk-" + strings.Repeat("x", 32)}
		},
		"workspace traversal": func(value *Manifest) {
			value.Mounts[0].Source = "../outside"
		},
		"overlapping mount": func(value *Manifest) {
			value.Mounts = append(value.Mounts,
				Mount{Source: "src", Target: "/workspace/src", Access: MountReadOnly})
		},
		"wildcard network": func(value *Manifest) {
			value.Network = NetworkScope{Mode: "allowlist", AllowedTargets: []string{"*.example.com"}}
		},
		"invalid DNS label": func(value *Manifest) {
			value.Network = NetworkScope{Mode: "allowlist", AllowedTargets: []string{"good.-bad.example"}}
		},
		"noncanonical CIDR": func(value *Manifest) {
			value.Network = NetworkScope{Mode: "allowlist", AllowedTargets: []string{"192.0.2.9/24"}}
		},
		"output on read only mount": func(value *Manifest) {
			value.Output.Paths = []string{"/workspace/result.json"}
		},
		"output equals mount root": func(value *Manifest) {
			value.Mounts[0].Access = MountReadWrite
			value.Output.Paths = []string{"/workspace"}
		},
		"unbounded timeout": func(value *Manifest) {
			value.TimeoutSeconds = MaxTimeoutSeconds + 1
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			value := validManifest()
			mutate(&value)
			if _, err := NormalizeManifest(value); err == nil {
				t.Fatalf("invalid manifest was accepted: %#v", value)
			}
		})
	}
}

func TestManifestValidatorsRemainFailClosed(t *testing.T) {
	manifest := validManifest()
	validated, err := NewNoopRunner().ValidateManifest(context.Background(), manifest)
	if err != nil || validated.ProtocolVersion != ManifestProtocolVersion {
		t.Fatalf("Noop validator failed: %#v err=%v", validated, err)
	}
	if _, err := NewLocalRunner().ValidateManifest(context.Background(), manifest); err == nil ||
		!strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Local validator did not fail closed: %v", err)
	}
	if _, err := NewDockerRunnerWithBinary("definitely-missing-cyberagent-docker").
		ValidateManifest(context.Background(), manifest); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Docker validator did not fail closed: %v", err)
	}
}

func validManifest() Manifest {
	return Manifest{
		ProtocolVersion: ManifestProtocolVersion,
		Backend:         BackendNoop,
		Command: CommandSpec{
			Executable: "go", Arguments: []string{"test", "./..."},
			WorkingDirectory: "/workspace",
		},
		Mounts:  []Mount{{Source: ".", Target: "/workspace", Access: MountReadOnly}},
		Network: NetworkScope{Mode: "disabled"},
		Resources: ResourceLimits{
			CPUQuotaMillis: 1000, MemoryBytes: 256 * 1024 * 1024,
			PIDs: 64, MaxOutputBytes: 4 * 1024 * 1024,
		},
		Output:         OutputSpec{CaptureStdout: true, CaptureStderr: true},
		TimeoutSeconds: 300,
		Cancellation:   CancellationSpec{GracePeriodMillis: 2000},
	}
}
