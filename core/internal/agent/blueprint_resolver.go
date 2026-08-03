// parlay-feature: parlay-tool/multi-adapter
// parlay-component: blueprint-scope-and-precedence
//
// Layered-setting resolver for the blueprint > adapter-set > adapter-default
// precedence chain. Scaffolding for that precedence feature, which is not yet
// wired into a production caller (no validator consults it today) — kept so the
// resolution semantics land with tests ahead of the feature.
//
// Note: canonical operation-error mapping is NOT a pre-codegen gate. Error
// representation (conflict -> ConflictException, P2002 -> conflict, etc.) is a
// codegen-time concern handled by each adapter's error-mapping conventions, so
// there is no structured error-mapping validator here.

package agent

// SourceLayer names the layer a resolved value came from. The closed set
// is {blueprint, adapter-set, adapter, default}. "default" applies when no
// layer declared the value and the resolver fell through to the static
// default for the key.
type SourceLayer string

const (
	SourceLayerBlueprint  SourceLayer = "blueprint"
	SourceLayerAdapterSet SourceLayer = "adapter-set"
	SourceLayerAdapter    SourceLayer = "adapter"
	SourceLayerDefault    SourceLayer = "default"
)

// LayeredSettings is the projection of the three layers a setting may
// inhabit. Each layer is a map keyed by setting name (e.g., "data.fetching",
// "auth.strategy"). Values are intentionally interface{} since settings
// span scalar types and small structs.
type LayeredSettings struct {
	Blueprint  map[string]interface{}
	AdapterSet map[string]interface{}
	Adapter    map[string]interface{}
}

// ResolveLayeredSetting returns the resolved value for the supplied key
// alongside the source layer that produced it. The precedence is
// blueprint > adapter-set > adapter; the default layer is reported when no
// declared value is found.
func ResolveLayeredSetting(layers LayeredSettings, key string) (interface{}, SourceLayer) {
	if v, ok := layers.Blueprint[key]; ok {
		return v, SourceLayerBlueprint
	}
	if v, ok := layers.AdapterSet[key]; ok {
		return v, SourceLayerAdapterSet
	}
	if v, ok := layers.Adapter[key]; ok {
		return v, SourceLayerAdapter
	}
	return nil, SourceLayerDefault
}
