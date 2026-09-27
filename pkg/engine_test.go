package pkg

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
)

// TestCreateAddendums_ShortHistory covers images whose history has fewer
// entries than layers (the normal case for cigno-built images, since layers
// are appended without history). All layers beyond the base must be included.
func TestCreateAddendums_ShortHistory(t *testing.T) {
	history := []v1.History{
		{CreatedBy: "base"},
	}
	// 3 layers but only 1 history entry: base=1 layer, 2 diff layers
	layers := make([]v1.Layer, 3)
	adds := createAddendums(1, 2, history, layers)
	if len(adds) != 2 {
		t.Fatalf("expected 2 addendums, got %d", len(adds))
	}
}

func TestCreateAddendums_CleanHistory(t *testing.T) {
	history := []v1.History{
		{CreatedBy: "base1"},
		{CreatedBy: "base2"},
		{CreatedBy: "app1"},
		{CreatedBy: "empty", EmptyLayer: true},
		{CreatedBy: "app2"},
	}
	layers := make([]v1.Layer, 4)
	adds := createAddendums(2, 3, history, layers)
	// diff = app1 layer, empty history entry, app2 layer
	if len(adds) != 3 {
		t.Fatalf("expected 3 addendums, got %d", len(adds))
	}
}

func TestSetEnvVars(t *testing.T) {
	cfg := &v1.ConfigFile{Config: v1.Config{
		Env: []string{"PATH=/usr/bin", "OLD=1"},
	}}
	err := setEnvVars(cfg, map[string]string{
		"PATH": "/data:/usr/bin",
		"NEW":  "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PATH": "/data:/usr/bin",
		"OLD":  "1",
		"NEW":  "x",
	}
	got := map[string]string{}
	for _, e := range cfg.Config.Env {
		kv := splitKV(e)
		got[kv[0]] = kv[1]
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env %s = %q, want %q (all: %v)", k, got[k], v, cfg.Config.Env)
		}
	}
}

func splitKV(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s, ""}
}

func TestExpandCurArg(t *testing.T) {
	engine := NewEngine()
	engine.GlobalArg["G"] = "global"
	engine.AddContext("", "scratch")
	stage := mkStage()
	engine.AllocLocalArgs(stage)
	engine.withArg(stage, "L", "local")

	engine.curStage = stage
	if got := engine.expandCurArg("$L-$G"); got != "local-global" {
		t.Errorf("expandCurArg = %q, want local-global", got)
	}
	// without a current stage, only global args expand
	engine.curStage = nil
	if got := engine.expandCurArg("$G"); got != "global" {
		t.Errorf("expandCurArg = %q, want global", got)
	}
}

func TestEngine_NilMapSafety(t *testing.T) {
	// a bare Engine{} (as done by external consumers) must not panic
	engine := Engine{}
	stage := mkStage()
	engine.AllocLocalArgs(stage)
	engine.WithGlobalArg("k", "v")
	engine.withArg(stage, "k2", "v2")
	if err := engine.doEnv(stage, mkEnvCmd("E", "1")); err != nil {
		t.Fatal(err)
	}
	engine.AllocLocalEnv(stage, []string{"A=1"})
}

func mkStage() *instructions.Stage {
	return &instructions.Stage{Name: "test", BaseName: "scratch"}
}

func mkEnvCmd(key, value string) *instructions.EnvCommand {
	return &instructions.EnvCommand{
		Env: instructions.KeyValuePairs{{Key: key, Value: value}},
	}
}
