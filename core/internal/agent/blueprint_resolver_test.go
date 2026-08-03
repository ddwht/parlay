// parlay-feature: parlay-tool/multi-adapter
// parlay-component: blueprint-scope-and-precedence
// parlay-artifact: test

package agent

import "testing"

func TestResolveLayeredSetting_BlueprintWins(t *testing.T) {
	layers := LayeredSettings{
		Blueprint:  map[string]interface{}{"data.fetching": "stale-while-revalidate"},
		AdapterSet: map[string]interface{}{"data.fetching": "on-mount"},
		Adapter:    map[string]interface{}{"data.fetching": "manual"},
	}
	v, src := ResolveLayeredSetting(layers, "data.fetching")
	if src != SourceLayerBlueprint {
		t.Errorf("source: got %q, want blueprint", src)
	}
	if v != "stale-while-revalidate" {
		t.Errorf("value: got %v", v)
	}
}

func TestResolveLayeredSetting_AdapterSetFallback(t *testing.T) {
	layers := LayeredSettings{
		AdapterSet: map[string]interface{}{"auth.strategy": "jwt"},
		Adapter:    map[string]interface{}{"auth.strategy": "session"},
	}
	v, src := ResolveLayeredSetting(layers, "auth.strategy")
	if src != SourceLayerAdapterSet {
		t.Errorf("source: got %q, want adapter-set", src)
	}
	if v != "jwt" {
		t.Errorf("value: got %v", v)
	}
}

func TestResolveLayeredSetting_DefaultLayer(t *testing.T) {
	layers := LayeredSettings{}
	_, src := ResolveLayeredSetting(layers, "missing.key")
	if src != SourceLayerDefault {
		t.Errorf("source: got %q, want default", src)
	}
}
