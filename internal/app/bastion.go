package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	oci_bastion "github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/core"

	"toci/internal/clients"
	"toci/internal/registry"
)

func listBastions(ctx context.Context, factory *clients.Factory, s registry.Scope) ([]pickerItem, error) {
	client, err := factory.Bastion(s.Region)
	if err != nil {
		return nil, err
	}
	resp, err := client.ListBastions(ctx, oci_bastion.ListBastionsRequest{CompartmentId: &s.CompartmentID})
	if err != nil {
		return nil, err
	}
	items := make([]pickerItem, 0, len(resp.Items))
	for _, b := range resp.Items {
		items = append(items, pickerItem{key: deref(b.Id), label: deref(b.Name)})
	}
	return items, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// instancePrivateIP resolves an instance's primary VNIC private IP, which
// the bastion session needs as its connection target.
func instancePrivateIP(ctx context.Context, factory *clients.Factory, s registry.Scope, instanceID string) (string, error) {
	compute, err := factory.Compute(s.Region)
	if err != nil {
		return "", err
	}
	attResp, err := compute.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
		CompartmentId: &s.CompartmentID,
		InstanceId:    &instanceID,
	})
	if err != nil {
		return "", err
	}
	var vnicID string
	for _, a := range attResp.Items {
		if a.LifecycleState == core.VnicAttachmentLifecycleStateAttached && a.VnicId != nil {
			vnicID = *a.VnicId
			break
		}
	}
	if vnicID == "" {
		return "", fmt.Errorf("instance has no attached VNIC")
	}

	vcn, err := factory.VirtualNetwork(s.Region)
	if err != nil {
		return "", err
	}
	vnicResp, err := vcn.GetVnic(ctx, core.GetVnicRequest{VnicId: &vnicID})
	if err != nil {
		return "", err
	}
	if vnicResp.PrivateIp == nil {
		return "", fmt.Errorf("VNIC has no private IP")
	}
	return *vnicResp.PrivateIp, nil
}

// sshKeyPair is one local key pair candidate for a bastion/direct session:
// the public key content (registered with the bastion session) and the
// private key path (substituted into the ssh command that connects with it).
type sshKeyPair struct {
	name           string // private key's filename under ~/.ssh — shown in the picker when there's more than one
	pubKeyContent  string
	privateKeyPath string
}

// listSSHKeyPairs finds every usable local SSH key pair in ~/.ssh: every
// "*.pub" file that has a matching private key alongside it (whatever it's
// named — e.g. an OCI console download like "ExascaleRAC-ssh-key-....key",
// not just the conventional id_ed25519/id_rsa/id_ecdsa). There's no way to
// tell from the filesystem alone which one a given target actually trusts,
// so the caller asks when there's more than one instead of guessing.
func listSSHKeyPairs() ([]sshKeyPair, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	pubFiles, err := filepath.Glob(filepath.Join(home, ".ssh", "*.pub"))
	if err != nil {
		return nil, err
	}
	sort.Strings(pubFiles)
	var pairs []sshKeyPair
	for _, pub := range pubFiles {
		priv := strings.TrimSuffix(pub, ".pub")
		if _, statErr := os.Stat(priv); statErr != nil {
			continue // a .pub with no private key alongside can't be connected with
		}
		content, readErr := os.ReadFile(pub)
		if readErr != nil {
			continue
		}
		pairs = append(pairs, sshKeyPair{name: filepath.Base(priv), pubKeyContent: strings.TrimSpace(string(content)), privateKeyPath: priv})
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("no SSH key pair found in ~/.ssh (looked for a *.pub file with a matching private key)")
	}
	return pairs, nil
}

// bastionSessionTTLSeconds is the TTL every bastion session is created
// with. Model.bastionSessions caches a created session's ssh command for
// reuse up to this same window (minus a safety margin — see
// bastionSessionReuseMargin), instead of creating a fresh session (and
// burning one of the bastion's limited concurrent-session slots) on every
// single reconnect to the same target+user.
const bastionSessionTTLSeconds = 1800

// bastionSessionCacheKey identifies a reusable session by exactly what OCI
// scopes a managed SSH session to: one bastion, one target instance, one OS
// user. A different port or IP isn't a dimension toci lets the user vary.
func bastionSessionCacheKey(bastionID, instanceID, username string) string {
	return bastionID + "|" + instanceID + "|" + username
}

// createBastionSession creates a managed-SSH bastion session and blocks
// until it becomes ACTIVE (or fails / times out). It's meant to run inside
// a tea.Cmd goroutine — sleeping here doesn't block the UI.
func createBastionSession(ctx context.Context, factory *clients.Factory, s registry.Scope, bastionID, instanceID, privateIP, username, pubKey string) (oci_bastion.Session, error) {
	client, err := factory.Bastion(s.Region)
	if err != nil {
		return oci_bastion.Session{}, err
	}

	ttl := bastionSessionTTLSeconds
	createResp, err := client.CreateSession(ctx, oci_bastion.CreateSessionRequest{
		CreateSessionDetails: oci_bastion.CreateSessionDetails{
			BastionId: &bastionID,
			TargetResourceDetails: oci_bastion.CreateManagedSshSessionTargetResourceDetails{
				TargetResourceId:                      &instanceID,
				TargetResourceOperatingSystemUserName: &username,
				TargetResourcePrivateIpAddress:        &privateIP,
				TargetResourcePort:                    intPtr(22),
			},
			KeyDetails:          &oci_bastion.PublicKeyDetails{PublicKeyContent: &pubKey},
			SessionTtlInSeconds: &ttl,
			DisplayName:         strPtr("toci-" + username),
		},
	})
	if err != nil {
		return oci_bastion.Session{}, err
	}

	sessionID := createResp.Session.Id
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		getResp, err := client.GetSession(ctx, oci_bastion.GetSessionRequest{SessionId: sessionID})
		if err != nil {
			return oci_bastion.Session{}, err
		}
		switch getResp.Session.LifecycleState {
		case oci_bastion.SessionLifecycleStateActive:
			return getResp.Session, nil
		case oci_bastion.SessionLifecycleStateFailed:
			return oci_bastion.Session{}, fmt.Errorf("bastion session failed to become active")
		}
		time.Sleep(3 * time.Second)
	}
	return oci_bastion.Session{}, fmt.Errorf("timed out waiting for bastion session to become active")
}

// buildSSHCommand turns a session's connection metadata into a runnable
// shell command, substituting the local private key path for the API's
// "<privateKey>" placeholder.
func buildSSHCommand(session oci_bastion.Session, privateKeyPath string) (string, error) {
	cmd, ok := session.SshMetadata["command"]
	if !ok {
		return "", fmt.Errorf("session has no ssh command metadata")
	}
	cmd = strings.ReplaceAll(cmd, "<privateKey>", privateKeyPath)
	// OCI's bastion network path drops an idle relay connection well before
	// the session's own TTL expires (independent timeout, not configurable
	// on our side) — periodic keepalives on both ssh hops (the outer
	// session and the ProxyCommand hop to the bastion, both start with
	// "ssh -i" in the template OCI returns) stop that from happening.
	cmd = strings.ReplaceAll(cmd, "ssh -i", "ssh -o ServerAliveInterval=60 -o ServerAliveCountMax=3 -i")
	return cmd, nil
}

// buildDirectSSHCommand builds a plain ssh command straight to the
// instance's private IP, bypassing the Bastion service — for networks
// (e.g. on-prem over FastConnect) that already reach the VCN directly.
func buildDirectSSHCommand(username, privateIP, privateKeyPath string) string {
	// ConnectTimeout: without a route to the target VCN (the whole reason
	// Bastion exists), the TCP handshake to a private IP just hangs with
	// no output at all rather than failing fast — this at least surfaces
	// a visible error instead of leaving the embedded terminal looking
	// frozen indefinitely.
	return fmt.Sprintf("ssh -o ConnectTimeout=8 -i %q %s@%s", privateKeyPath, username, privateIP)
}

func intPtr(n int) *int       { return &n }
func strPtr(s string) *string { return &s }
