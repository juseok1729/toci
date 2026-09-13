package registry

import (
	"context"
	"strings"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// osAbbrev maps OCI's own verbose Image.OperatingSystem values (there's no
// short form in the API itself) to the shorthand people actually use, so
// the OS column doesn't eat half the table width. Anything not listed here
// (CentOS Linux, a Marketplace image's own OS string, ...) falls back to
// the raw value in shortOS below — still readable, just not abbreviated.
var osAbbrev = map[string]string{
	"Oracle Linux":            "OL",
	"Oracle Autonomous Linux": "OAL",
	"Canonical Ubuntu":        "Ubuntu",
	"Windows":                 "Win",
}

// shortOS abbreviates os per osAbbrev, or returns it unchanged if it isn't
// one of the known long forms.
func shortOS(os string) string {
	if short, ok := osAbbrev[os]; ok {
		return short
	}
	return os
}

// versionAbbrev shortens the handful of long words OCI's own
// OperatingSystemVersion string uses for Windows images ("Server 2019
// Standard") — Linux versions ("8.9", "22.04") are already short and never
// match any of these. Applied word-by-word in shortVersion.
var versionAbbrev = map[string]string{
	"Server":     "Svr",
	"Standard":   "Std",
	"Datacenter": "DC",
	"Enterprise": "Ent",
}

// shortVersion abbreviates each word of version per versionAbbrev, leaving
// anything not listed (version numbers, "R2", ...) untouched.
func shortVersion(version string) string {
	words := strings.Fields(version)
	for i, w := range words {
		if short, ok := versionAbbrev[w]; ok {
			words[i] = short
		}
	}
	return strings.Join(words, " ")
}

// imageLabel joins an image's (already-abbreviated) OS and version into
// one label, e.g. "OL 8.9" — except a custom image, where OCI itself sets
// *both* OperatingSystem and OperatingSystemVersion to the literal string
// "Custom" (it has no way to know the actual guest OS), which would
// otherwise join into "Custom Custom". Skipping the version whenever it
// duplicates the OS handles that case without special-casing "Custom" by
// name — any other OS/version pair that happened to match would be
// deduplicated the same way.
func imageLabel(os, version string) string {
	label := shortOS(os)
	if v := shortVersion(version); v != "" && v != label {
		if label != "" {
			label += " "
		}
		label += v
	}
	return label
}

// fetchImageLabels resolves each distinct ImageId among instances to a
// short "OS Version" label (e.g. "OL 8.9", "Ubuntu 22.04") via GetImage —
// there's no bulk lookup, but a compartment's instances typically share
// only a handful of distinct images, so this is a handful of calls
// regardless of instance count. A failed lookup (deleted/inaccessible
// image) just leaves that image's label blank rather than failing the
// whole listing.
func fetchImageLabels(ctx context.Context, client core.ComputeClient, instances []core.Instance) map[string]string {
	imageIDs := make(map[string]bool)
	for _, i := range instances {
		if id := deref(i.ImageId); id != "" {
			imageIDs[id] = true
		}
	}

	type result struct {
		imageID string
		label   string
	}
	results := make(chan result, len(imageIDs))
	var wg sync.WaitGroup
	for imageID := range imageIDs {
		wg.Add(1)
		go func(imageID string) {
			defer wg.Done()
			resp, err := client.GetImage(ctx, core.GetImageRequest{ImageId: &imageID})
			if err != nil {
				return
			}
			results <- result{imageID, imageLabel(deref(resp.OperatingSystem), deref(resp.OperatingSystemVersion))}
		}(imageID)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	out := make(map[string]string, len(imageIDs))
	for r := range results {
		out[r.imageID] = r.label
	}
	return out
}
