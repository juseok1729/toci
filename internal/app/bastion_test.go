package app

import (
	"os"
	"strings"
	"testing"

	oci_bastion "github.com/oracle/oci-go-sdk/v65/bastion"
)

func TestBuildChainedSSHCommand(t *testing.T) {
	session := oci_bastion.Session{Id: strPtr("ocid1.bastionsession.oc1..aaa")}

	cmd, err := buildChainedSSHCommand(session, "ap-seoul-1", "10.16.0.22", "10.18.0.52", "opc", "/home/user/.ssh/key")
	if err != nil {
		t.Fatalf("buildChainedSSHCommand() error = %v", err)
	}
	if !strings.HasPrefix(cmd, "ssh -F ") || !strings.HasSuffix(cmd, " toci-target") {
		t.Fatalf("cmd = %q, want \"ssh -F <path> toci-target\" shape", cmd)
	}
	path := strings.TrimSuffix(strings.TrimPrefix(cmd, "ssh -F "), " toci-target")
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	config := string(content)

	for _, want := range []string{
		"Host toci-bastion",
		"HostName host.bastion.ap-seoul-1.oci.oraclecloud.com",
		"User ocid1.bastionsession.oc1..aaa",
		"Host toci-jump",
		"HostName 10.16.0.22",
		"ProxyJump toci-bastion",
		"Host toci-target",
		"HostName 10.18.0.52",
		"ProxyJump toci-jump",
		"IdentityFile /home/user/.ssh/key",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("generated config missing %q:\n%s", want, config)
		}
	}
	// jump and target hops both authenticate as the picked OS username.
	if strings.Count(config, "User opc") != 2 {
		t.Errorf("expected \"User opc\" twice (jump + target hops), got:\n%s", config)
	}
}

func TestBuildChainedSSHCommandNoSessionID(t *testing.T) {
	if _, err := buildChainedSSHCommand(oci_bastion.Session{}, "ap-seoul-1", "10.16.0.22", "10.18.0.52", "opc", "/key"); err == nil {
		t.Fatal("expected an error for a session with no Id")
	}
}
