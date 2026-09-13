package app

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// recentEntry is one MRU slot in the "Recent:" header line (F2) — enough to
// render the header and jump straight back without the compartment tree
// even having loaded yet (path/name are cached here for that reason,
// alongside the tree cache's own copy).
type recentEntry struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Subtree bool   `yaml:"subtree"`
	Pinned  bool   `yaml:"pinned"`
}

// recentMaxSlots is F2's cap: 1-9 header hotkeys.
const recentMaxSlots = 9

// recentConfig is the on-disk shape: recent compartments are tenancy
// (profile) specific, so each profile gets its own list.
type recentConfig struct {
	Profiles map[string][]recentEntry `yaml:"profiles"`
}

func recentConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "toci", "recent.yaml"), nil
}

// loadRecent reads this profile's saved MRU list, or nil if there is none
// yet (first run, or a config dir that isn't writable).
func loadRecent(profile string) []recentEntry {
	path, err := recentConfigPath()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg recentConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil
	}
	return cfg.Profiles[profile]
}

// saveRecent persists profile's MRU list, best-effort — a failure here
// (read-only home, no config dir) shouldn't interrupt using the app.
func saveRecent(profile string, entries []recentEntry) {
	path, err := recentConfigPath()
	if err != nil {
		return
	}
	cfg := recentConfig{Profiles: map[string][]recentEntry{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(b, &cfg)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string][]recentEntry{}
	}
	cfg.Profiles[profile] = entries

	b, err := yaml.Marshal(cfg)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}

// pushRecent records a visit to (id, name) as the new MRU head, preserving
// pinned entries regardless of how long ago they were last visited (doc
// F2: "pin 하면 MRU에서 밀리지 않는다"). subtree is the mode the compartment
// was visited in — shown as a "⊕" suffix on the header hotkey.
func pushRecent(entries []recentEntry, id, name string, subtree bool) []recentEntry {
	out := make([]recentEntry, 0, len(entries)+1)
	out = append(out, recentEntry{ID: id, Name: name, Subtree: subtree})
	for _, e := range entries {
		if e.ID == id {
			continue // superseded by the new head above
		}
		out = append(out, e)
	}
	return trimRecent(out)
}

// togglePin flips id's pinned flag, inserting it at the MRU head first if
// it isn't already tracked (picking "p" on a compartment the user hasn't
// visited yet still pins it).
func togglePin(entries []recentEntry, id, name string) []recentEntry {
	for i, e := range entries {
		if e.ID == id {
			entries[i].Pinned = !entries[i].Pinned
			return trimRecent(entries)
		}
	}
	out := append([]recentEntry{{ID: id, Name: name, Pinned: true}}, entries...)
	return trimRecent(out)
}

// trimRecent caps the list at recentMaxSlots, dropping unpinned entries
// from the tail first — a pinned entry is only ever displaced by running
// out of room for other pinned entries, which recentMaxSlots (9) makes
// vanishingly unlikely in practice.
func trimRecent(entries []recentEntry) []recentEntry {
	if len(entries) <= recentMaxSlots {
		return entries
	}
	var pinned, unpinned []recentEntry
	for _, e := range entries {
		if e.Pinned {
			pinned = append(pinned, e)
		} else {
			unpinned = append(unpinned, e)
		}
	}
	room := recentMaxSlots - len(pinned)
	if room < 0 {
		room = 0
	}
	if len(unpinned) > room {
		unpinned = unpinned[:room]
	}
	out := make([]recentEntry, 0, len(pinned)+len(unpinned))
	out = append(out, pinned...)
	out = append(out, unpinned...)
	return out
}
