package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// NL2SQLGenerator defines the interface for converting natural language to SQL
type NL2SQLGenerator interface {
	GenerateSQL(schema, question string) (string, error)
}

// MockNL2SQLGenerator implements NL2SQLGenerator for testing
type MockNL2SQLGenerator struct{}

func (m *MockNL2SQLGenerator) GenerateSQL(schema, question string) (string, error) {
	// For testing, just return a hardcoded SQL query based on the question
	if question == "Any hotel in Basel?" {
		return "SELECT * FROM hotels WHERE city = 'Basel'", nil
	}
	return "SELECT * FROM hotels LIMIT 5", nil
}

// PostgresPool represents a connection pool to a PostgreSQL database
type PostgresPool struct {
	// In a real implementation, this would contain the actual connection pool
}

// Query executes a SQL query and returns the results
func (p *PostgresPool) Query(ctx context.Context, sql string) (*Rows, error) {
	// Mock implementation that returns fake data
	if sql == "SELECT * FROM hotels WHERE city = 'Basel'" {
		return &Rows{
			data: []map[string]interface{}{
				{
					"id":              1,
					"name":            "Grand Hotel Basel",
					"city":            "Basel",
					"stars":           4,
					"price_per_night": 250.00,
				},
				{
					"id":              2,
					"name":            "Basel Budget Inn",
					"city":            "Basel",
					"stars":           3,
					"price_per_night": 120.50,
				},
			},
			columns: []FieldDescription{
				{Name: []byte("id")},
				{Name: []byte("name")},
				{Name: []byte("city")},
				{Name: []byte("stars")},
				{Name: []byte("price_per_night")},
			},
		}, nil
	}
	
	return &Rows{
		data: []map[string]interface{}{
			{
				"id":              3,
				"name":            "Zurich Luxury Hotel",
				"city":            "Zurich",
				"stars":           5,
				"price_per_night": 350.00,
			},
		},
		columns: []FieldDescription{
			{Name: []byte("id")},
			{Name: []byte("name")},
			{Name: []byte("city")},
			{Name: []byte("stars")},
			{Name: []byte("price_per_night")},
		},
	}, nil
}

// FieldDescription represents a column in a database result set
type FieldDescription struct {
	Name []byte
}

// Rows represents the result of a database query
type Rows struct {
	data    []map[string]interface{}
	columns []FieldDescription
	current int
}

// Next advances to the next row
func (r *Rows) Next() bool {
	r.current++
	return r.current <= len(r.data)
}

// Scan copies the current row's column values into the provided destination variables
func (r *Rows) Scan(dest ...interface{}) error {
	if r.current < 1 || r.current > len(r.data) {
		return fmt.Errorf("invalid row index")
	}
	
	row := r.data[r.current-1]
	for i, d := range dest {
		colName := string(r.columns[i].Name)
		switch v := d.(type) {
		case *interface{}:
			*v = row[colName]
		default:
			return fmt.Errorf("unsupported scan destination")
		}
	}
	return nil
}

// FieldDescriptions returns the column descriptions
func (r *Rows) FieldDescriptions() []FieldDescription {
	return r.columns
}

// Close closes the rows
func (r *Rows) Close() {}

// Err returns any error that occurred while iterating
func (r *Rows) Err() error {
	return nil
}

// PostgresSource represents a PostgreSQL data source
type PostgresSource struct {
	pool   *PostgresPool
	schema string
}

// PostgresPool returns the connection pool
func (s *PostgresSource) PostgresPool() *PostgresPool {
	return s.pool
}

// PostgresSchema returns the database schema
func (s *PostgresSource) PostgresSchema() string {
	return s.schema
}

// PostgresNLConfig represents the configuration for a PostgresNL tool
type PostgresNLConfig struct {
	Name        string
	Kind        string
	Source      string
	Description string
}

// PostgresNLTool implements a tool that converts natural language to SQL for PostgreSQL
type PostgresNLTool struct {
	config    PostgresNLConfig
	source    *PostgresSource
	generator NL2SQLGenerator
}

// Invoke processes a natural language question and returns SQL results
func (t *PostgresNLTool) Invoke(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Extract the natural language question from parameters
	questionParam, ok := params["question"]
	if !ok {
		return nil, fmt.Errorf("missing required parameter 'question'")
	}

	question, ok := questionParam.(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'question' must be a string")
	}

	// Get the schema from the source
	schema := t.source.PostgresSchema()

	// Generate SQL from the natural language question
	sql, err := t.generator.GenerateSQL(schema, question)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SQL: %v", err)
	}

	// Execute the generated SQL query
	rows, err := t.source.PostgresPool().Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to execute SQL query: %v", err)
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

	// Return the results along with the generated SQL
	return map[string]interface{}{
		"sql":     sql,
		"results": results,
	}, nil
}

func main() {
	fmt.Println("PostgresNL Tool Standalone Test")
	fmt.Println("===============================")

	// Create a mock source
	source := &PostgresSource{
		pool: &PostgresPool{},
		schema: `
CREATE TABLE hotels (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  city VARCHAR(50) NOT NULL,
  stars INTEGER,
  price_per_night DECIMAL(10, 2)
);`,
	}

	// Create a mock generator
	generator := &MockNL2SQLGenerator{}

	// Create a tool config
	config := PostgresNLConfig{
		Name:        "natural-language-question-to-db",
		Kind:        "postgres-nl",
		Source:      "mock-source",
		Description: "Convert natural language questions to SQL queries for PostgreSQL",
	}

	// Create the tool
	tool := &PostgresNLTool{
		config:    config,
		source:    source,
		generator: generator,
	}

	// Test the tool with a question
	fmt.Println("\nTesting with question: 'Any hotel in Basel?'")
	params := map[string]interface{}{
		"question": "Any hotel in Basel?",
	}

	result, err := tool.Invoke(context.Background(), params)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Pretty print the result
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println("Result:")
	fmt.Println(string(resultJSON))

	fmt.Println("\nTest completed successfully!")
}