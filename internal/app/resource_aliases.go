package app

// resourceSearchAliases are extra search terms for the ":" resource
// search, on top of a resource's own Label() — the industry-standard OCI
// abbreviations users already know (e.g. "DBCS" for DB Systems, "FSS" for
// File Systems) rather than the spelled-out label they'd have to guess.
// Keyed by Key() like resourceDescriptions, and likewise only covering
// resource kinds this app actually implements — LPG, RPC, OCIR, Block
// Volume, IAM, IDCS, and WAF are real OCI
// abbreviations too, just not ones toci has a resource for yet.
var resourceSearchAliases = map[string][]string{
	"security-list":   {"SL"},
	"nsg":             {"NSG"},
	"drg":             {"DRG"},
	"service-gateway": {"SG"},
	"nat-gateway":     {"NAT"},
	"igw":             {"IGW"},
	// Instances covers both VM and bare-metal shapes — either abbreviation
	// should find it.
	"instance":    {"VM", "BM"},
	"oke":         {"OKE"},
	"bucket":      {"OS"}, // Object Storage
	"file-system": {"FSS"},
	// Autonomous Database is one resource covering both the ADW (data
	// warehouse) and ATP (transaction processing) workload types — either
	// abbreviation should find it, alongside ADB itself.
	"adb":       {"ADB", "ADW", "ATP"},
	"db-system": {"DBCS"},
	"mysql":     {"MDS", "HeatWave"}, // MDS = MySQL Database Service, its old name
	"exadata":   {"ExaCS"},
}
