// Copyright 2024 Google LLC
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
package postgresnl

import (
	"context"
	"fmt"
	"os"

	"github.com/googleapis/genai-toolbox/internal/sources"
	"github.com/googleapis/genai-toolbox/internal/sources/postgres"
	"github.com/googleapis/genai-toolbox/internal/tools"
	"github.com/googleapis/genai-toolbox/internal/tools/nl2sql"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ToolKind string = "postgres-nl"

type compatibleSource interface {
	PostgresPool() *pgxpool.Pool
}

// validate compatible sources are still compatible
var _ compatibleSource = &postgres.Source{}


var compatibleSources = [...]string{postgres.SourceKind}

type Config struct {
	Name        string           `yaml:"name" validate:"required"`
	Kind        string           `yaml:"kind" validate:"required"`
	Source      string           `yaml:"source" validate:"required"`
	Description string           `yaml:"description" validate:"required"`
	Parameters  tools.Parameters `yaml:"parameters"`
}


// validate interface
var _ tools.ToolConfig = Config{}
func (c Config) GetName() string {
	return c.Name
}

func (c Config) GetKind() string {
	return c.Kind
}

func (c Config) GetSource() string {
	return c.Source
}

func (c Config) GetDescription() string {
	return c.Description
}

func (c Config) GetParameters() tools.Parameters {
	return c.Parameters
}

func (c Config) ToolConfigKind() string {
	return ToolKind
}

func (c Config) Initialize(sourceMap map[string]sources.Source) (tools.Tool, error) {
	// Verify source exists and is compatible
	source, ok := sourceMap[c.Source]
	if !ok {
		return nil, fmt.Errorf("source %q not found", c.Source)
	}

	compatSource, ok := source.(compatibleSource)
	if !ok {
		return nil, fmt.Errorf("source %q is not compatible with postgres-nl tool", c.Source)
	}

	// Get Gemini API key from environment
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	// Create NL2SQL generator
	generator := nl2sql.NewGeminiPostgreSQLGenerator(apiKey)

	// Create and return the tool
	return &Tool{
		config:    c,
		source:    compatSource,
		generator: generator,
	}, nil
}

type Tool struct {
	config    Config
	source    compatibleSource
	generator nl2sql.NL2SQLGenerator
}
func (t *Tool) GetID() string {
	return t.config.Name
}

func (t *Tool) GetDescription() string {
	return t.config.Description
}

func (t *Tool) GetParameters() tools.Parameters {
	return t.config.Parameters
}

// Authorized implements the tools.Tool interface
func (t *Tool) Authorized(users []string) bool {
	// By default, all users are authorized to use this tool
	return true
}

// Manifest implements the tools.Tool interface
func (t *Tool) Manifest() tools.Manifest {
	return tools.Manifest{
		Description: t.GetDescription(),
		Parameters:  t.GetParameters().Manifest(),
	}
}

// ParseParams implements the tools.Tool interface
func (t *Tool) ParseParams(data map[string]any, claims map[string]map[string]any) (tools.ParamValues, error) {
	return tools.ParseParams(t.GetParameters(), data, claims)
}

// Invoke implements the tools.Tool interface
func (t *Tool) Invoke(params tools.ParamValues) ([]any, error) {
	ctx := context.Background()

	// Convert params to a map for easier access
	paramsMap := params.AsMap()

	// Get the question parameter
	questionValue, ok := paramsMap["question"]
	if !ok {
		return nil, fmt.Errorf("missing required parameter 'question'")
	}

	question, ok := questionValue.(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'question' must be a string")
	}
	// Get the schema from the source by querying the database
	pool := t.source.PostgresPool()

	// Query to get table schema information
	schemaQuery := `
		SELECT
			table_name,
			column_name,
			data_type
		FROM
			information_schema.columns
		WHERE
			table_schema = 'public'
		ORDER BY
			table_name,
			ordinal_position;
	`

	schemaRows, err := pool.Query(ctx, schemaQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query database schema: %v", err)
	}
	defer schemaRows.Close()

	// Build schema string
	schema := ""
	currentTable := ""

	for schemaRows.Next() {
		var tableName, columnName, dataType string
		if err := schemaRows.Scan(&tableName, &columnName, &dataType); err != nil {
			return nil, fmt.Errorf("failed to scan schema row: %v", err)
		}

		if tableName != currentTable {
			if currentTable != "" {
				schema += ");\n\n"
			}
			currentTable = tableName
			schema += fmt.Sprintf("CREATE TABLE %s (\n", tableName)
		} else {
			schema += ",\n"
		}

		schema += fmt.Sprintf("  %s %s", columnName, dataType)
	}

	if currentTable != "" {
		schema += ");\n"
	}

	if err := schemaRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating schema rows: %v", err)
	}

	// Generate SQL from the natural language question
	sql, err := t.generator.GenerateSQL(ctx, schema, question)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SQL: %v", err)
	}

	// Execute the generated SQL query
	rows, err := t.source.PostgresPool().Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to execute SQL query %s: %v", sql, err)
	}
	defer rows.Close()

	// Process the results
	var results []map[string]interface{}
	for rows.Next() {
		// Get column descriptions
		colDescs := rows.FieldDescriptions()
		values := make([]interface{}, len(colDescs))
		valuePtrs := make([]interface{}, len(colDescs))

		// Create a slice of pointers to the values
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan the row into the slice of pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}

		// Create a map for the row
		row := make(map[string]interface{})
		for i, colDesc := range colDescs {
			row[string(colDesc.Name)] = values[i]
		}

		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %v", err)
	}

	// Return the results along with the generated SQL as a slice of any
	return []any{
		map[string]interface{}{
			// "sql":     sql,
			"results": results,
		},
	}, nil
}
