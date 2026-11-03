// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package extensions_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/googleapis/mcp-toolbox/internal/extensions"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgressql"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/pkg/extension"
)

// recorder records the calls it is handed and lets them through. It serves
// HTTP, takes new configurations, and counts how often it is closed.
type recorder struct {
	config map[string]any
	calls  []extension.Call
	closed int
}

func (r *recorder) Intercept(ctx context.Context, c extension.Call, next extension.Next) (any, error) {
	r.calls = append(r.calls, c)
	return next(ctx)
}

func (r *recorder) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { fmt.Fprintf(w, "path %s", req.URL.Path) })
}

func (r *recorder) Reconfigure(_ context.Context, cfg map[string]any, _ extension.Host) error {
	r.config = cfg
	return nil
}

func (r *recorder) Close(context.Context) error {
	r.closed++
	return nil
}

// plain implements no optional interface.
type plain struct{}

var created []*recorder

func init() {
	extension.Register("test-recorder", func(_ context.Context, _ string, cfg map[string]any, _ extension.Host) (any, error) {
		r := &recorder{config: cfg}
		created = append(created, r)
		return r, nil
	})
	extension.Register("test-plain", func(context.Context, string, map[string]any, extension.Host) (any, error) {
		return plain{}, nil
	})
}

const twoTools = `
kind: tool
name: cancel
type: postgres-sql
source: db
description: Cancel an order.
parameters:
  - {name: id, type: integer, description: The order.}
statement: UPDATE orders SET status = 'cancelled' WHERE id = $1
%s
---
kind: tool
name: list
type: postgres-sql
source: db
description: List orders.
statement: SELECT * FROM orders
`

func parse(t *testing.T, yaml string) (server.ToolConfigs, extensions.Configs) {
	t.Helper()
	ctx := context.Background()
	_, _, _, tcs, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, []byte(yaml))
	if err != nil {
		t.Fatalf("UnmarshalPrimitiveConfig: %v", err)
	}
	ecs, err := server.UnmarshalExtensionConfigs(ctx, []byte(yaml))
	if err != nil {
		t.Fatalf("UnmarshalExtensionConfigs: %v", err)
	}
	return tcs, ecs
}

func TestNoExtensionsLeavesConfigsAsTheyAre(t *testing.T) {
	tcs, ecs := parse(t, fmt.Sprintf(twoTools, ""))
	if ecs != nil {
		t.Errorf("extension configs: %v, want nil", ecs)
	}
	for name, tc := range tcs {
		if _, ok := tc.(postgressql.Config); !ok {
			t.Errorf("tool %s: config is %T, want postgressql.Config", name, tc)
		}
	}
}

func TestParse(t *testing.T) {
	yaml := "kind: extension\nname: review\ntype: test-recorder\nlevel: 3\n---\n" +
		fmt.Sprintf(twoTools, "extensions:\n  review: {policy: writes}")
	tcs, ecs := parse(t, yaml)
	if diff := cmp.Diff(extensions.Configs{"review": {Type: "test-recorder", Config: map[string]any{"level": uint64(3)}}}, ecs); diff != "" {
		t.Errorf("extension configs (-want +got):\n%s", diff)
	}
	ec, ok := tcs["cancel"].(extensions.ToolConfig)
	if !ok {
		t.Fatalf("cancel: config is %T, want extensions.ToolConfig", tcs["cancel"])
	}
	if diff := cmp.Diff(map[string]map[string]any{"review": {"policy": "writes"}}, ec.Options); diff != "" {
		t.Errorf("cancel: options (-want +got):\n%s", diff)
	}
	if _, ok := ec.ToolConfig.(postgressql.Config); !ok {
		t.Errorf("cancel: inner config is %T", ec.ToolConfig)
	}
	if _, ok := tcs["list"].(postgressql.Config); !ok {
		t.Errorf("list: a tool without extensions is %T, want postgressql.Config", tcs["list"])
	}
}

func TestUnknownType(t *testing.T) {
	if _, err := server.UnmarshalExtensionConfigs(context.Background(), []byte("kind: extension\nname: x\ntype: nope\n")); err == nil {
		t.Fatal("want an error for a type no compiled-in extension registered")
	}
}

// fakeTool runs nothing; it returns "ran".
type fakeTool struct {
	tools.Tool
	cfg tools.ToolConfig
}

func (f fakeTool) Invoke(context.Context, sources.Source, parameters.ParamValues, tools.AccessToken) (any, util.ToolboxError) {
	return "ran", nil
}
func (f fakeTool) ToConfig() tools.ToolConfig { return f.cfg }
func (f fakeTool) GetSourceName() string      { return "db" }
func (f fakeTool) GetDescription() string     { return "Cancel an order." }

func isFake(t tools.Tool) bool {
	_, ok := t.(fakeTool)
	return ok
}

func TestWrappedToolSeesRenderedSQL(t *testing.T) {
	ctx := context.Background()
	defer extensions.CloseAll(ctx)
	tcs, ecs := parse(t, "kind: extension\nname: review\ntype: test-recorder\n---\n"+fmt.Sprintf(twoTools, "extensions: {review: {}}"))
	g, err := extensions.Start(ctx, ecs, nil)
	if err != nil {
		t.Fatal(err)
	}
	inner := tcs["cancel"].(extensions.ToolConfig).ToolConfig
	wrapped, err := g.Wrap("cancel", tcs["cancel"], fakeTool{cfg: inner}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain, _ := g.Wrap("list", tcs["list"], fakeTool{cfg: tcs["list"]}, nil); !isFake(plain) {
		t.Errorf("list: wrapped as %T, want the tool itself", plain)
	}
	out, terr := wrapped.Invoke(ctx, nil, parameters.ParamValues{{Name: "id", Value: 7}}, "")
	if terr != nil || out != "ran" {
		t.Fatalf("Invoke = %v, %v; want ran (the extension calls next)", out, terr)
	}
	r := created[len(created)-1]
	want := []extension.Call{{
		Tool: "cancel", ToolType: "postgres-sql", Description: "Cancel an order.", Source: "db",
		Args: map[string]any{"id": 7}, Options: map[string]any{},
		Statements: []extension.Statement{{SQL: "UPDATE orders SET status = 'cancelled' WHERE id = $1", Args: []extension.Arg{{Type: "integer", Value: 7}}}},
	}}
	if diff := cmp.Diff(want, r.calls); diff != "" {
		t.Errorf("calls (-want +got):\n%s", diff)
	}
}

func TestToolNamingAnExtensionThatTakesNoCalls(t *testing.T) {
	ctx := context.Background()
	defer extensions.CloseAll(ctx)
	tcs, ecs := parse(t, "kind: extension\nname: p\ntype: test-plain\n---\n"+fmt.Sprintf(twoTools, "extensions: {p: {}}"))
	g, err := extensions.Start(ctx, ecs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Wrap("cancel", tcs["cancel"], fakeTool{}, nil); err == nil {
		t.Error("want an error: extension p does not implement Interceptor")
	}
	if _, err := g.Wrap("cancel", extensions.ToolConfig{ToolConfig: tcs["list"], Options: map[string]map[string]any{"missing": {}}}, fakeTool{}, nil); err == nil {
		t.Error("want an error for an extension name with no `kind: extension`")
	}
}

func TestLifecycleAcrossReloads(t *testing.T) {
	ctx := context.Background()
	created = nil
	if _, err := extensions.Start(ctx, extensions.Configs{"r": {Type: "test-recorder", Config: map[string]any{"v": 1}}}, nil); err != nil {
		t.Fatal(err)
	}
	// Reload with a new configuration: the same instance takes it.
	if _, err := extensions.Start(ctx, extensions.Configs{"r": {Type: "test-recorder", Config: map[string]any{"v": 2}}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || created[0].config["v"] != 2 {
		t.Fatalf("after reload: %d instances, config %v; want 1 instance reconfigured to v=2", len(created), created[0].config)
	}
	// Served under /extensions/r/.
	rec := httptest.NewRecorder()
	extensions.HTTPHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/extensions/r/cs/1", nil))
	if got := rec.Body.String(); got != "path /cs/1" {
		t.Errorf("GET /extensions/r/cs/1 = %q, want the extension's handler with the prefix stripped", got)
	}
	rec = httptest.NewRecorder()
	extensions.HTTPHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/extensions/nobody/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /extensions/nobody/ = %d, want 404", rec.Code)
	}
	// Removed from the configuration: closed.
	if _, err := extensions.Start(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	if created[0].closed != 1 {
		t.Errorf("removed instance closed %d times, want 1", created[0].closed)
	}
}
