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

package nl2sql

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// NL2SQLGenerator defines the interface for converting natural language to SQL
type NL2SQLGenerator interface {
	// GenerateSQL converts a natural language question to SQL using the provided database schema
	GenerateSQL(ctx context.Context, dbSchema string, question string) (string, error)
}

// GeminiPostgreSQLGenerator implements NL2SQLGenerator using Google's Gemini API
type GeminiPostgreSQLGenerator struct {
	APIKey string
}

// NewGeminiPostgreSQLGenerator creates a new GeminiPostgreSQLGenerator
func NewGeminiPostgreSQLGenerator(apiKey string) *GeminiPostgreSQLGenerator {
	return &GeminiPostgreSQLGenerator{
		APIKey: apiKey,
	}
}

// GenerateSQL converts a natural language question to SQL using Gemini
func (g *GeminiPostgreSQLGenerator) GenerateSQL(ctx context.Context, dbSchema string, question string) (string, error) {
	// Create Gemini client
	client, err := genai.NewClient(ctx, option.WithAPIKey(g.APIKey))
	if err != nil {
		return "", fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	// Configure the model
	model := client.GenerativeModel("gemini-2.0-flash")
	model.SetTemperature(0.4)
	model.SetMaxOutputTokens(500)
	model.SetTopP(0.8)
	model.SetTopK(40)

	// Create the prompt
	prompt := fmt.Sprintf(
		"You are a SQL expert, given the database schema %s, generate a query that will answer the question: %s. For canceling a booking request, return query like this \"UPDATE hotels SET booked = B'0' WHERE id = 1;\". Return the executable query text only without any other comments or quotes.",
		dbSchema, question)

	// Generate the SQL
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("Gemini API call failed: %w", err)
	}

	// Extract the SQL from the response
	sql, err := extractTextFromResponse(resp)
	sql = strings.ReplaceAll(sql, "```sql", "")
	sql = strings.ReplaceAll(sql, "```", "")
	if err != nil {
		return "", err
	}

	return sql, nil
}

// extractTextFromResponse extracts text content from a Gemini response
func extractTextFromResponse(resp *genai.GenerateContentResponse) (string, error) {
	if resp == nil || len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}

	var result strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			result.WriteString(string(text))
		}
	}

	return result.String(), nil
}