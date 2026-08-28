package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// localSSHKeyPair finds a usable local SSH key pair, preferring modern
// ed25519 keys over rsa. The bastion session is registered with the public
// key; the private key path is substituted into the connection command the
// API returns.
func localSSHKeyPair() (pubKeyContent, privateKeyPath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		priv := filepath.Join(home, ".ssh", name)
		pub := priv + ".pub"
		content, readErr := os.ReadFile(pub)
		if readErr != nil {
			continue
		}
		return strings.TrimSpace(string(content)), priv, nil
	}
	return "", "", fmt.Errorf("no SSH key pair found in ~/.ssh (looked for id_ed25519, id_rsa, id_ecdsa)")
}

// createBastionSession creates a managed-SSH bastion session and blocks
// until it becomes ACTIVE (or fails / times out). It's meant to run inside
// a tea.Cmd goroutine — sleeping here doesn't block the UI.
func createBastionSession(ctx context.Context, factory *clients.Factory, s registry.Scope, bastionID, instanceID, privateIP, username, pubKey string) (oci_bastion.Session, error) {
	client, err := factory.Bastion(s.Region)
	if err != nil {
		return oci_bastion.Session{}, err
	}

	ttl := 1800
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
	return strings.ReplaceAll(cmd, "<privateKey>", privateKeyPath), nil
}

func intPtr(n int) *int       { return &n }
func strPtr(s string) *string { return &s }
