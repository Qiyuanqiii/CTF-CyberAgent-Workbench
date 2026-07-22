package browserruntime

import "testing"

func TestTargetScopeCanonicalizesAndAuthorizesExactOrigins(t *testing.T) {
	scope, err := NewTargetScope(ProfileSafeWeb, []string{
		"HTTPS://Example.COM/path?q=1", "https://example.com:443/other", "http://localhost:3000/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Origins) != 2 || scope.Origins[0].String() != "http://localhost:3000" ||
		scope.Origins[1].String() != "https://example.com:443" {
		t.Fatalf("unexpected canonical origins: %#v", scope.Origins)
	}
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}

	allowed := scope.AuthorizeNavigation("https://EXAMPLE.com/account?tab=one")
	if !allowed.Allowed || allowed.Code != "allowed" ||
		allowed.CanonicalURL != "https://example.com:443/account?tab=one" ||
		!allowed.ResolutionCheckRequired {
		t.Fatalf("unexpected navigation decision: %#v", allowed)
	}
	if decision := scope.AuthorizeNavigation("https://example.com.evil.test/"); decision.Allowed || decision.Code != "origin_not_allowed" {
		t.Fatalf("off-origin redirect passed: %#v", decision)
	}
	if decision := scope.AuthorizeNavigation("http://example.com/"); decision.Allowed {
		t.Fatalf("scheme-changing redirect passed: %#v", decision)
	}
}

func TestTargetScopeSeparatesSafeAndCTFPrivateAccess(t *testing.T) {
	privateTargets := []string{"http://192.168.56.10:8080", "http://10.0.0.5"}
	if _, err := NewTargetScope(ProfileSafeWeb, privateTargets); err == nil {
		t.Fatal("safe-web accepted private network targets")
	}
	scope, err := NewTargetScope(ProfileCTFLab, privateTargets)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Origins) != 2 {
		t.Fatalf("unexpected private scope: %#v", scope.Origins)
	}
}

func TestTargetScopeAlwaysBlocksMetadataAndUnsafeSchemes(t *testing.T) {
	blocked := []string{
		"http://169.254.169.254/latest/meta-data", "http://169.254.170.2/",
		"http://100.100.100.200/", "http://168.63.129.16/",
		"http://metadata.google.internal/", "file:///etc/passwd", "data:text/plain,hello",
		"http://user:secret@example.com/", "http://example.com/#fragment",
		"http://0.0.0.0/", "http://224.0.0.1/", "http://example.com\\@evil.test/",
	}
	for _, target := range blocked {
		if _, err := NewTargetScope(ProfileCTFInstrumented, []string{target}); err == nil {
			t.Fatalf("blocked target %q passed", target)
		}
	}
}

func TestResolvedAddressRevalidationBlocksRebinding(t *testing.T) {
	safe, err := NewTargetScope(ProfileSafeWeb, []string{"https://challenge.example"})
	if err != nil {
		t.Fatal(err)
	}
	origin := safe.Origins[0]
	for _, address := range []string{"127.0.0.1", "10.0.0.9", "169.254.169.254", "fd00:ec2::254"} {
		if decision := safe.AuthorizeResolvedAddress(origin, address); decision.Allowed {
			t.Fatalf("safe-web accepted rebound address %s: %#v", address, decision)
		}
	}
	if decision := safe.AuthorizeResolvedAddress(origin, "93.184.216.34"); !decision.Allowed {
		t.Fatalf("safe-web rejected public address: %#v", decision)
	}

	lab, err := NewTargetScope(ProfileCTFLab, []string{"https://challenge.example"})
	if err != nil {
		t.Fatal(err)
	}
	if decision := lab.AuthorizeResolvedAddress(lab.Origins[0], "10.0.0.9"); !decision.Allowed {
		t.Fatalf("ctf-lab rejected explicitly scoped private resolution: %#v", decision)
	}
	if decision := lab.AuthorizeResolvedAddress(lab.Origins[0], "169.254.169.254"); decision.Allowed {
		t.Fatalf("ctf-lab accepted metadata address: %#v", decision)
	}
}

func TestLiteralAddressCannotChangeAfterApproval(t *testing.T) {
	scope, err := NewTargetScope(ProfileCTFLab, []string{"http://127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	origin := scope.Origins[0]
	if decision := scope.AuthorizeResolvedAddress(origin, "127.0.0.1"); !decision.Allowed {
		t.Fatalf("exact literal rejected: %#v", decision)
	}
	if decision := scope.AuthorizeResolvedAddress(origin, "127.0.0.2"); decision.Allowed ||
		decision.Code != "literal_address_mismatch" {
		t.Fatalf("changed literal passed: %#v", decision)
	}
}

func TestTargetScopeTamperingFailsClosed(t *testing.T) {
	scope, err := NewTargetScope(ProfileCTFLab, []string{"http://192.168.1.10"})
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*TargetScope){
		func(value *TargetScope) { value.Authority.NetworkAccess = true },
		func(value *TargetScope) { value.DefaultDeny = false },
		func(value *TargetScope) { value.RedirectRevalidation = false },
		func(value *TargetScope) { value.Origins[0].Host = "192.168.1.11" },
		func(value *TargetScope) { value.Fingerprint = "00" },
	}
	for index, mutate := range mutations {
		candidate := scope
		candidate.Origins = append([]Origin(nil), scope.Origins...)
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("mutation %d unexpectedly passed", index)
		}
	}
}

func FuzzTargetScopeParsing(f *testing.F) {
	for _, seed := range []string{
		"https://example.com/path?q=1", "http://127.0.0.1:8080", "http://[::1]:3000",
		"file:///etc/passwd", "http://user:secret@example.com", "http://169.254.169.254/",
		"http://example.com\\@127.0.0.1/", "\x00", "http://例子.测试/",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, rawTarget string) {
		scope, err := NewTargetScope(ProfileCTFInstrumented, []string{rawTarget})
		if err != nil {
			return
		}
		if err := scope.Validate(); err != nil {
			t.Fatalf("accepted scope failed reconstruction: %v", err)
		}
		decision := scope.AuthorizeNavigation(rawTarget)
		if !decision.Allowed {
			t.Fatalf("accepted target failed authorization: %#v", decision)
		}
	})
}
