# SSE Parser

This package provides functionality for parsing server-sent event streams,
defined by [the SSE
specification](https://html.spec.whatwg.org/multipage/server-sent-events.html).

## Usage

In this example, we make a streaming chat completion request to the OpenAI
platform API, and scan for response chunks:

```go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jclem/sseparser"
)

func main() {
	openaiKey := os.Getenv("OPENAI_KEY")

	if openaiKey == "" {
		log.Fatal("OPENAI_KEY environment variable must be set")
	}

	params := map[string]interface{}{
		"model":  "gpt-3.5-turbo",
		"stream": true,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Hello, how are you?",
			},
		},
	}

	body, err := json.Marshal(params)
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+openaiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode != 200 {
		log.Fatal(resp.Status)
	}

	defer resp.Body.Close()

	// We create a new stream scanner that reads the HTTP response body.
	scanner := sseparser.NewStreamScanner(resp.Body)

	for {
		// Then, we call `UnmarshalNext`, and log each completion chunk, until we
		// encounter an error or reach the end of the stream.
		var e event
		_, err := scanner.UnmarshalNext(&e)
		if err != nil {
			if errors.Is(err, sseparser.ErrStreamEOF) {
				os.Exit(0)
			}

			log.Fatal(err)
		}

		if len(e.Data.Choices) > 0 {
			fmt.Print(e.Data.Choices[0].Delta.Content)
		}
	}
}

// The event struct uses tags to specify how to unmarshal the event data.
type event struct {
	Data chunk `sse:"data"`
}

type chunk struct {
	Choices []choice
}

// The chunk struct implements the `sseparser.UnmarshalerSSEValue` interface,
// which makes it easy to unmarshal event field values which in this case are
// complete JSON payloads.
func (c *chunk) UnmarshalSSEValue(v string) error {
	if v == "[DONE]" {
		return nil
	}

	return json.Unmarshal([]byte(v), c)
}

type choice struct {
	Delta delta
}

type delta struct {
	Content string
}
```