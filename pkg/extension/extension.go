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

// Package extension lets a Go module outside this repository add
// experimental features to Toolbox. The module registers an extension
// type in init() and is compiled into a Toolbox binary whose main imports
// it:
//
//	import (
//		"github.com/googleapis/mcp-toolbox/cmd"
//		_ "example.com/my-extension"
//	)
//
//	func main() { cmd.Execute() }
//
// A `kind: extension` document in tools.yaml creates an instance; the rest
// of the document is the extension's configuration. A tool hands its
// calls to an instance with an `extensions:` field, keyed by instance name,
// with options for that instance:
//
//	kind: extension
//	name: review
//	type: my-extension
//	---
//	kind: tool
//	name: cancel-order
//	type: postgres-sql
//	extensions:
//	  review: {policy: order-writes}
//
// What an instance does is the set of optional interfaces it implements:
// Interceptor, HTTPHandler, Reconfigurer, Closer. An extension module may
// also add tool types with RegisterTool.
//
// A configuration without `kind: extension` documents runs exactly as
// before, and tools without `extensions:` are never handed to one.
//
// EXPERIMENTAL: this API may change in any release.
package extension

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/spanner"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Factory creates an instance from its `kind: extension` document (the
// document without kind, name and type). It returns the instance; Toolbox
// uses whichever optional interfaces below the instance implements.
type Factory func(ctx context.Context, name string, config map[string]any, host Host) (any, error)

// Interceptor runs around the calls of the tools that name the instance in
// their `extensions:` field. It may call next, return its own result
// without calling next, or return an error.
type Interceptor interface {
	Intercept(ctx context.Context, c Call, next Next) (any, error)
}

// HTTPHandler serves HTTP under /extensions/<name>/ on Toolbox's address;
// the prefix is stripped. The handler does its own authentication.
type HTTPHandler interface {
	HTTPHandler() http.Handler
}

// Reconfigurer takes a new configuration when tools.yaml is reloaded.
// Without it, Toolbox closes the instance and creates a new one.
type Reconfigurer interface {
	Reconfigure(ctx context.Context, config map[string]any, host Host) error
}

// Closer releases an instance when Toolbox shuts down or the instance is
// removed from tools.yaml.
type Closer interface {
	Close(ctx context.Context) error
}

// Host is the Toolbox server an instance runs in. A reload gives the
// instance a new Host.
type Host interface {
	// Source returns an initialized source. Use the interfaces below
	// (PostgresPooler and so on) to reach its connection pool or client.
	Source(name string) (any, bool)
	// Invoke runs a tool with the given arguments without handing it to
	// any extension: for a call an Interceptor held and now lets through.
	Invoke(ctx context.Context, tool string, args map[string]any) (any, error)
}

// Call is one tool call handed to an Interceptor.
type Call struct {
	Tool        string
	ToolType    string
	Description string
	Source      string // the tool's source, if it has one
	SourceType  string
	Args        map[string]any
	// Statements is the SQL the tool would run, rendered from its
	// configuration and Args, for the SQL tools Toolbox can render; nil
	// for other tools.
	Statements []Statement
	// Options is the tool's options for this instance, from its
	// `extensions:` field.
	Options map[string]any
}

// Next runs the call as Toolbox would without the extension.
type Next func(ctx context.Context) (any, error)

// Statement is a SQL statement a tool would run, with its arguments bound
// the way the tool binds them: positionally ($1, ?) when Name is empty,
// by name (@name) otherwise.
type Statement struct {
	SQL  string
	Args []Arg
}

// Arg is one bound argument. Type is the tool parameter's type, when known.
type Arg struct {
	Name  string
	Type  string
	Value any
}

// The connection pools and clients of the SQL sources. Each source type
// that implements one asserts it in a test, so a rename breaks the build
// here, not an extension.
type (
	PostgresPooler  interface{ PostgresPool() *pgxpool.Pool }
	MySQLPooler     interface{ MySQLPool() *sql.DB }
	SpannerClienter interface {
		SpannerClient() *spanner.Client
		DatabaseDialect() string
	}
	BigQueryClienter interface{ BigQueryClient() *bigquery.Client }
)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register registers an extension type. It panics if the type is already
// registered.
func Register(typ string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := factories[typ]; dup {
		panic(fmt.Sprintf("extension type %q already registered", typ))
	}
	factories[typ] = f
}

// Lookup returns the factory for an extension type.
func Lookup(typ string) (Factory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := factories[typ]
	return f, ok
}
