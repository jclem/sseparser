# SSE Parser

This package provides functionality for parsing server-sent event streams,
defined by [the SSE
specification](https://html.spec.whatwg.org/multipage/server-sent-events.html).

**Note:** This is currently pretty basic to cover some personal needs. It does
not handle malformed streams well. This includes issues like panicking when
parsing incomplete fields, for example.

## Usage

```go
input := []byte(":comment\nkey1: value1\nkey2: value2\n\n")

stream, err := sse.Parse(bytes.NewReader(input))
if err != nil {
    panic(err)
}

for _, event := range stream {
  for _, comment := range event.Comments() {
    fmt.Println(comment)
  }

  for _, field := range event.Fields() {
    fmt.Printf("%s: %s\n", field.Key, field.Value)
  }
}
```