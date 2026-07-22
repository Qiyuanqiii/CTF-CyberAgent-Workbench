package browserruntime

import "testing"

func TestBuiltinProfilesRemainFixedAndNonAuthorizing(t *testing.T) {
	profiles := BuiltinRegistry().List()
	if len(profiles) != 3 || profiles[0].ID != ProfileCTFInstrumented ||
		profiles[1].ID != ProfileCTFLab || profiles[2].ID != ProfileSafeWeb {
		t.Fatalf("unexpected profile order: %#v", profiles)
	}
	for _, profile := range profiles {
		if err := ValidateProfileDescriptor(profile); err != nil {
			t.Fatalf("validate %s: %v", profile.ID, err)
		}
		if profile.Authority != (RuntimeAuthority{}) {
			t.Fatalf("profile %s unexpectedly grants authority", profile.ID)
		}
		if !profile.Isolation.DisposableProfileRequired || !profile.Isolation.PersonalProfileForbidden ||
			!profile.Network.RequireExactOrigins || !profile.Network.RevalidateRedirects ||
			!profile.Network.RevalidateResolvedIPs {
			t.Fatalf("profile %s lost a mandatory boundary", profile.ID)
		}
	}

	safe, _ := BuiltinRegistry().Lookup(ProfileSafeWeb)
	if !safe.Network.AllowLoopbackTargets || safe.Network.AllowPrivateTargets ||
		safe.Tools.RequestMutation || safe.Security.MayRelaxOriginPolicy || safe.ApprovalRequired {
		t.Fatalf("safe-web controls widened: %#v", safe)
	}
	lab, _ := BuiltinRegistry().Lookup(ProfileCTFLab)
	if !lab.Network.AllowPrivateTargets || !lab.Tools.RequestInterception ||
		!lab.Tools.RequestMutation || !lab.Tools.RequestReplay ||
		lab.Security.MayRelaxOriginPolicy || !lab.ApprovalRequired {
		t.Fatalf("ctf-lab controls are incomplete: %#v", lab)
	}
	instrumented, _ := BuiltinRegistry().Lookup(ProfileCTFInstrumented)
	if !instrumented.Isolation.ContainerRequired || !instrumented.Security.MayRelaxOriginPolicy ||
		!instrumented.Security.MayRelaxMixedContent ||
		!instrumented.Security.MayRelaxCertificateErrors ||
		instrumented.EvidenceClass != "instrumented" {
		t.Fatalf("ctf-instrumented controls are incomplete: %#v", instrumented)
	}
}

func TestProfileValidationRejectsTampering(t *testing.T) {
	profile, _ := BuiltinRegistry().Lookup(ProfileCTFLab)
	originalDigest, err := ProfileFingerprint(profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(originalDigest) != 64 {
		t.Fatalf("unexpected profile digest %q", originalDigest)
	}

	mutations := []func(*ProfileDescriptor){
		func(value *ProfileDescriptor) { value.Authority.NetworkAccess = true },
		func(value *ProfileDescriptor) { value.Isolation.PersonalProfileForbidden = false },
		func(value *ProfileDescriptor) { value.Network.BlockCloudMetadata = false },
		func(value *ProfileDescriptor) { value.Limits.MaxRequests++ },
		func(value *ProfileDescriptor) { value.Security.MayRelaxOriginPolicy = true },
	}
	for index, mutate := range mutations {
		candidate := profile
		mutate(&candidate)
		if err := ValidateProfileDescriptor(candidate); err == nil {
			t.Fatalf("mutation %d unexpectedly passed", index)
		}
	}
}

func TestParseProfileIDIsClosed(t *testing.T) {
	for _, value := range []string{"safe-web", " CTF-LAB ", "ctf-instrumented"} {
		if _, err := ParseProfileID(value); err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "unsafe", "ctf-lab-extra"} {
		if _, err := ParseProfileID(value); err == nil {
			t.Fatalf("unsupported profile %q passed", value)
		}
	}
}
