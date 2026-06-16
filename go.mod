module github.com/ddwht/parlay

go 1.25.0

require (
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/core-cli-wiring
//
// In-process dependency on the studio module so the validate-vocabulary
// CLI command can call vocabulary.Validate / vocabulary.LoadFromAdapterFile
// directly. The replace directive pins the actual sources to the sibling
// ./studio directory (local-only wiring). When studio publishes a tagged
// release the replace can be dropped.
require github.com/parlay-tool/parlay/studio v0.0.0

replace github.com/parlay-tool/parlay/studio => ./studio
