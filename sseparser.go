// Package sseparser provides a parser for Server-Sent Events (SSE).
// The SSE specification this package is modeled on can be found here:
// https://html.spec.whatwg.org/multipage/server-sent-events.html
//
// The primary means of utilizing this package is through the [StreamScanner]
// type, which scans an [io.Reader] for SSEs.
package sseparser

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"

	parsec "github.com/prataprc/goparsec"
)

var lf = parsec.AtomExact("\u000A", "LF")
var cr = parsec.AtomExact("\u000D", "CR")
var spa = parsec.AtomExact("\u0020", "SP")
var col = parsec.AtomExact("\u003A", "COLON")
var bom = parsec.AtomExact("\uFEFF", "BOM")

var namechar = parsec.TokenExact("(?:[\u0000-\u0009]|[\u000B-\u000C]|[\u000E-\u0039]|[\u003B-\U0010FFFF])", "NAMECHAR")
var anychar = parsec.TokenExact("(?:[\u0000-\u0009]|[\u000B-\u000C]|[\u000E-\U0010FFFF])", "ANYCHAR")
var eol = parsec.OrdChoice(nil, parsec.And(nil, cr, lf), cr, lf)

// Field is an SSE field: A field name and an optional value.
type Field struct {
	// Name is the field name.
	Name string
	// Value is the field value. This may be an empty string.
	Value string
}

// ParseField parses a byte slice into a [Field].
func ParseField(input []byte) (Field, error) {
	scanner := parsec.NewScanner(input)
	node, scanner := fieldParser(scanner)
	if !scanner.Endof() {
		cursor := scanner.GetCursor()
		remainder := input[cursor:]
		return Field{}, fmt.Errorf("Unexpected input: %q", remainder)
	}

	return node.(Field), nil
}

var fieldParser = parsec.And(toField,
	parsec.Many(toString, namechar),
	parsec.Kleene(nil,
		parsec.And(nil,
			col,
			parsec.Maybe(nil, spa),
			parsec.Kleene(toString, anychar),
		),
	),
	eol,
)

func toField(nodes []parsec.ParsecNode) parsec.ParsecNode {
	name := nodes[0].(string)
	vnodes := nodes[1].([]parsec.ParsecNode)

	if len(vnodes) == 0 {
		return Field{name, ""}
	}

	v := vnodes[0].([]parsec.ParsecNode)[2]

	return Field{name, v.(string)}
}

// Comment is a comment in an SSE.
type Comment string

// ParseComment parses a byte slice into a [Comment].
func ParseComment(input []byte) (Comment, error) {
	scanner := parsec.NewScanner(input)
	node, scanner := commentParser(scanner)
	if !scanner.Endof() {
		cursor := scanner.GetCursor()
		remainder := input[cursor:]
		return Comment(""), fmt.Errorf("Unexpected input: %q", remainder)
	}

	return node.(Comment), nil
}

var commentParser = parsec.And(toComment,
	col,
	parsec.Kleene(toString, anychar),
	eol,
)

func toComment(nodes []parsec.ParsecNode) parsec.ParsecNode {
	str := nodes[1].(string)
	return Comment(str)
}

// Event is a server-sent event (SSE), which is a set of zero or more comments
// and/or fields.
//
// The underlying type is a []any, but each value will be either a [Field] or a
// [Comment].
type Event []any

// Fields returns the fields in an SSE.
func (e Event) Fields() []Field {
	fields := make([]Field, 0, len(e))

	for _, item := range e {
		if field, ok := item.(Field); ok {
			fields = append(fields, field)
		}
	}

	return fields
}

// Comments returns the comments in an SSE.
func (e Event) Comments() []Comment {
	comments := make([]Comment, 0, len(e))

	for _, item := range e {
		if comment, ok := item.(Comment); ok {
			comments = append(comments, comment)
		}
	}

	return comments
}

// UnmarshalerSSE is an interface implemented by types into which an SSE can be
// unmarshaled.
type UnmarshalerSSE interface {
	// UnmarshalSSE unmarshals the given event into the type.
	UnmarshalSSE(event Event) error
}

// UnmarshalerSSEValue is an interface implemented by types into which an SSE
// field value can be unmarshaled.
//
// This is useful for custom unmarshaling of field values, such as when a field
// value contains a complete JSON payload.
type UnmarshalerSSEValue interface {
	// UnmarshalSSEValue unmarshals the given event field value into the type.
	UnmarshalSSEValue(value string) error
}

func (e Event) unmarshal(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("invalid type: expected a non-nil pointer: %T", v)
	}

	if unmarshaler, ok := rv.Interface().(UnmarshalerSSE); ok {
		return unmarshaler.UnmarshalSSE(e)
	}

	// Deref to get the underlying value.
	rv = rv.Elem()

	if rv.Kind() != reflect.Struct {
		return errors.New("invalid type: expected a struct")
	}

	for i := 0; i < rv.NumField(); i++ {
		field := rv.Type().Field(i)

		// Look for "sse" tag.
		tag, ok := field.Tag.Lookup("sse")
		if !ok {
			continue
		}

		for _, eventField := range e.Fields() {
			if eventField.Name == tag {
				switch rv.Field(i).Kind() {
				case reflect.String:
					rv.Field(i).SetString(eventField.Value)

				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					intValue, err := strconv.Atoi(eventField.Value)
					if err != nil {
						return fmt.Errorf("failed to convert string to int: %v", err)
					}
					rv.Field(i).SetInt(int64(intValue))

				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					uintValue, err := strconv.ParseUint(eventField.Value, 10, 64)
					if err != nil {
						return fmt.Errorf("failed to convert string to uint: %v", err)
					}
					rv.Field(i).SetUint(uintValue)

				case reflect.Float32, reflect.Float64:
					floatValue, err := strconv.ParseFloat(eventField.Value, 64)
					if err != nil {
						return fmt.Errorf("failed to convert string to float: %v", err)
					}
					rv.Field(i).SetFloat(floatValue)

				case reflect.Bool:
					boolValue, err := strconv.ParseBool(eventField.Value)
					if err != nil {
						return fmt.Errorf("failed to convert string to bool: %v", err)
					}
					rv.Field(i).SetBool(boolValue)

				default:
					if unmarshaler, ok := rv.Field(i).Addr().Interface().(UnmarshalerSSEValue); ok {
						if err := unmarshaler.UnmarshalSSEValue(eventField.Value); err != nil {
							return fmt.Errorf("failed to unmarshal using custom UnmarshalSSEValue: %v", err)
						}
					}
				}

				break
			}
		}
	}

	return nil
}

// ParseEvent parses an input into an Event.
func ParseEvent(input []byte) (Event, error) {
	scanner := parsec.NewScanner(input)
	node, scanner := eventParser(scanner)
	if !scanner.Endof() {
		cursor := scanner.GetCursor()
		remainder := input[cursor:]
		return Event{}, fmt.Errorf("Unexpected input: %q", remainder)
	}

	return node.(Event), nil
}

var eventParser = parsec.And(toEvent,
	parsec.Kleene(nil,
		parsec.OrdChoice(toEventItem, commentParser, fieldParser),
	),
	eol,
)

func toEvent(nodes []parsec.ParsecNode) parsec.ParsecNode {
	eventItems := nodes[0].([]parsec.ParsecNode)
	event := Event(make([]any, 0, len(eventItems)))

	for _, node := range nodes[0].([]parsec.ParsecNode) {
		switch t := node.(type) {
		case Field:
			event = append(event, t)
		case Comment:
			event = append(event, t)
		default:
			panic(fmt.Sprintf("Unknown type: %T\n", t))
		}
	}

	return event
}

func toEventItem(nodes []parsec.ParsecNode) parsec.ParsecNode {
	node := nodes[0]

	switch t := node.(type) {
	case Field:
		return t
	case Comment:
		return t
	default:
		panic(fmt.Sprintf("Unknown type: %T\n", t))
	}
}

// Stream is a SSE stream, which is a set of zero or more [Event].
type Stream []Event

// ParseStream parses a byte slice into a [Stream].
func ParseStream(input []byte) (Stream, error) {
	scanner := parsec.NewScanner(input)
	node, scanner := streamParser(scanner)
	if !scanner.Endof() {
		cursor := scanner.GetCursor()
		remainder := input[cursor:]
		return Stream{}, fmt.Errorf("Unexpected input: %q", remainder)
	}

	return node.(Stream), nil
}

var streamParser = parsec.And(toStream,
	parsec.Maybe(nil, bom),
	parsec.Kleene(nil, eventParser),
)

func toStream(nodes []parsec.ParsecNode) parsec.ParsecNode {
	eventNodes := nodes[1].([]parsec.ParsecNode)
	stream := Stream(make([]Event, 0, len(eventNodes)))

	for _, node := range eventNodes {
		stream = append(stream, node.(Event))
	}

	return stream
}

func toString(nodes []parsec.ParsecNode) parsec.ParsecNode {
	var str string

	for _, node := range nodes {
		str += node.(*parsec.Terminal).Value
	}

	return str
}

// StreamScanner scans an [io.Reader] for SSEs.
type StreamScanner struct {
	buf []byte
	r   io.Reader
	rs  int
}

// ErrStreamEOF is returned when the end of the stream is reached. This error
// wraps an [io.EOF] error.
type ErrStreamEOF struct {
	eof error
}

// Error implements the error interface.
func (e ErrStreamEOF) Error() string {
	return "end of stream"
}

// Unwrap implements the error unwrapping interface.
func (e ErrStreamEOF) Unwrap() error {
	return e.eof
}

// newErrStreamEOF creates a new [ErrStreamEOF] error.
func newErrStreamEOF(err error) ErrStreamEOF {
	if err != io.EOF {
		panic("newErrStreamEOF called with non-EOF error")
	}

	return ErrStreamEOF{err}
}

// Next returns the next event in the stream. There are three possible return states:
//
//  1. An [Event], a byte slice, and nil are returned if an event was parsed.
//  2. nil, a byte slice, and [ErrStreamEOF] are returned if the end of the
//     stream was reached.
//  3. nil, a byte slice, and an error are returned if an error occurred while
//     reading from the stream.
//
// In all three cases, the byte slice contains any data that was read from the
// reader but was not part of the event. This data can be ignored while making
// subsequent calls to Next, but may be used to recover from errors, or just
// when not scanning the full stream.
func (s *StreamScanner) Next() (*Event, []byte, error) {
	for {
		b := make([]byte, s.rs)

		var eof error
		n, err := s.r.Read(b)
		s.buf = append(s.buf, b[:n]...)
		if err != nil {
			if err == io.EOF {
				eof = newErrStreamEOF(err)
			} else {
				return nil, s.buf, err
			}
		}

		scanner := parsec.NewScanner(s.buf)
		node, scanner := eventParser(scanner)

		if node, ok := node.(Event); ok {
			offset := scanner.GetCursor()
			s.buf = s.buf[offset:]
			return &node, s.buf, nil
		} else if eof != nil {
			return nil, s.buf, eof
		} else {
			continue
		}
	}
}

// UnmarshalNext unmarshals the next event in the stream into the provided
// struct. See [StreamScanner.Next] for details on the []byte and error return
// values.
func (s *StreamScanner) UnmarshalNext(v any) ([]byte, error) {
	event, left, err := s.Next()
	if err != nil {
		return left, err
	}

	return left, event.unmarshal(v)
}

// NewStreamScanner scans a [io.Reader] for SSEs.
func NewStreamScanner(reader io.Reader) *StreamScanner {
	return &StreamScanner{
		buf: []byte{},
		r:   reader,
		rs:  64,
	}
}
