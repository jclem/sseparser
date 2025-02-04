// Package sseparser provides a parser for Server-Sent Events (SSE).
// The SSE specification this package is modeled on can be found here:
// https://html.spec.whatwg.org/multipage/server-sent-events.html
//
// The primary means of utilizing this package is through the [StreamScanner]
// type, which scans an [io.Reader] for SSEs.
package sseparser

import (
	"encoding/json"
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
		return Field{}, fmt.Errorf("unexpected input: %q", remainder)
	}

	if field, ok := node.(Field); ok {
		return field, nil
	}

	return Field{}, fmt.Errorf("failed to parse field: unexpected type: %T", node)
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

func toField(nodes []parsec.ParsecNode) parsec.ParsecNode { //nolint:ireturn // Required by parsec
	name, ok := nodes[0].(string)
	if !ok {
		panic(fmt.Sprintf("unexpected type: %T\n", nodes[0]))
	}

	vnodes, ok := nodes[1].([]parsec.ParsecNode)
	if !ok {
		panic(fmt.Sprintf("unexpected type: %T\n", nodes[1]))
	}

	if len(vnodes) == 0 {
		return Field{name, ""}
	}

	vand, ok := vnodes[0].([]parsec.ParsecNode)
	if !ok {
		panic(fmt.Sprintf("unexpected type: %T\n", vnodes[0]))
	}

	v := vand[2]

	if vstr, ok := v.(string); ok {
		return Field{name, vstr}
	}

	panic(fmt.Sprintf("unexpected type: %T\n", v))
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
		return Comment(""), fmt.Errorf("unexpected input: %q", remainder)
	}

	c, ok := node.(Comment)
	if !ok {
		return Comment(""), fmt.Errorf("failed to parse comment: unexpected type: %T", node)
	}

	return c, nil
}

var commentParser = parsec.And(toComment,
	col,
	parsec.Kleene(toString, anychar),
	eol,
)

func toComment(nodes []parsec.ParsecNode) parsec.ParsecNode { //nolint: ireturn
	if str, ok := nodes[1].(string); ok {
		return Comment(str)
	}

	panic(fmt.Sprintf("unexpected type: %T\n", nodes[1]))
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

func unmarshalEvent(e Event, v any) error { //nolint:cyclop,funlen,gocognit // Easier as one large function
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("invalid type: expected a non-nil pointer: %T", v)
	}

	if unmarshaler, ok := rv.Interface().(UnmarshalerSSE); ok {
		if err := unmarshaler.UnmarshalSSE(e); err != nil {
			return fmt.Errorf("failed to unmarshal using UnmarshalerSSE interface: %w", err)
		}
	}

	// Deref to get the underlying value.
	rv = rv.Elem()

	if rv.Kind() != reflect.Struct {
		return errors.New("invalid type: expected a struct")
	}

	for i := range rv.NumField() {
		field := rv.Type().Field(i)

		isJSON := false

		tag, ok := field.Tag.Lookup("sse")
		if !ok {
			tag, ok = field.Tag.Lookup("ssejson")
			if !ok {
				continue
			}
			isJSON = true
		}

		value := ""
		for _, eventField := range e.Fields() {
			if eventField.Name == tag {
				value += eventField.Value
			}
		}

		if isJSON {
			v := rv.Field(i).Addr().Interface()
			if err := json.Unmarshal([]byte(value), &v); err != nil {
				return fmt.Errorf("unmarshal field JSON: %w", err)
			}

			continue
		}

		//nolint:exhaustive
		switch rv.Field(i).Kind() {
		case reflect.String:
			rv.Field(i).SetString(value)

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			intValue, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("failed to convert string to int: %w", err)
			}
			rv.Field(i).SetInt(int64(intValue))

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			uintValue, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to convert string to uint: %w", err)
			}
			rv.Field(i).SetUint(uintValue)

		case reflect.Float32, reflect.Float64:
			floatValue, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("failed to convert string to float: %w", err)
			}
			rv.Field(i).SetFloat(floatValue)

		case reflect.Bool:
			boolValue, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("failed to convert string to bool: %w", err)
			}
			rv.Field(i).SetBool(boolValue)

		default:
			if unmarshaler, ok := rv.Field(i).Addr().Interface().(UnmarshalerSSEValue); ok {
				if err := unmarshaler.UnmarshalSSEValue(value); err != nil {
					return fmt.Errorf("failed to unmarshal using custom UnmarshalSSEValue: %w", err)
				}
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
		return nil, fmt.Errorf("unexpected input: %q", remainder)
	}

	if n, ok := node.(Event); ok {
		return n, nil
	}

	return nil, fmt.Errorf("failed to parse event: unexpected type: %T", node)
}

var eventParser = parsec.And(toEvent,
	parsec.Kleene(nil,
		parsec.OrdChoice(toEventItem, commentParser, fieldParser),
	),
	eol,
)

func toEvent(nodes []parsec.ParsecNode) parsec.ParsecNode { //nolint: ireturn
	enodes, ok := nodes[0].([]parsec.ParsecNode)
	if !ok {
		panic(fmt.Sprintf("unexpected type: %T\n", nodes[0]))
	}

	event := Event(make([]any, 0, len(enodes)))

	for _, node := range enodes {
		switch t := node.(type) {
		case Field:
			event = append(event, t)
		case Comment:
			event = append(event, t)
		default:
			panic(fmt.Sprintf("unexpected type: %T\n", t))
		}
	}

	return event
}

func toEventItem(nodes []parsec.ParsecNode) parsec.ParsecNode { //nolint: ireturn
	node := nodes[0]

	switch t := node.(type) {
	case Field:
		return t
	case Comment:
		return t
	default:
		panic(fmt.Sprintf("unexpected type: %T\n", t))
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
		return Stream{}, fmt.Errorf("unexpected input: %q", remainder)
	}

	if s, ok := node.(Stream); ok {
		return s, nil
	}

	return nil, fmt.Errorf("failed to parse stream: unexpected type: %T", node)
}

var streamParser = parsec.And(toStream,
	parsec.Maybe(nil, bom),
	parsec.Kleene(nil, eventParser),
)

func toStream(nodes []parsec.ParsecNode) parsec.ParsecNode { //nolint: ireturn
	eventNodes, ok := nodes[1].([]parsec.ParsecNode)
	if !ok {
		panic(fmt.Sprintf("unexpected type: %T\n", nodes[1]))
	}

	stream := Stream(make([]Event, 0, len(eventNodes)))

	for _, node := range eventNodes {
		enode, ok := node.(Event)
		if !ok {
			panic(fmt.Sprintf("unexpected type: %T\n", node))
		}

		stream = append(stream, enode)
	}

	return stream
}

func toString(nodes []parsec.ParsecNode) parsec.ParsecNode { //nolint: ireturn
	var str string

	for _, node := range nodes {
		term, ok := node.(*parsec.Terminal)
		if !ok {
			panic(fmt.Sprintf("unexpected type: %T\n", node))
		}

		str += term.Value
	}

	return str
}

// StreamScanner scans an [io.Reader] for SSEs.
type StreamScanner struct {
	buf []byte
	r   io.Reader
	rs  int
}

// ErrStreamEOF is returned when the end of the stream is reached. It wraps
// [io.EOF].
var ErrStreamEOF = streamEOFError{io.EOF}

type streamEOFError struct {
	eof error
}

// Error implements the error interface.
func (e streamEOFError) Error() string {
	return "end of stream"
}

// Unwrap implements the error unwrapping interface.
func (e streamEOFError) Unwrap() error {
	return e.eof
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
func (s *StreamScanner) Next() (Event, []byte, error) {
	for {
		b := make([]byte, s.rs)

		var eof error
		n, err := s.r.Read(b)
		s.buf = append(s.buf, b[:n]...)
		if err != nil {
			if err != io.EOF {
				return nil, s.buf, fmt.Errorf("failed to read from reader: %w", err)
			}

			eof = ErrStreamEOF
		}

		scanner := parsec.NewScanner(s.buf)
		node, scanner := eventParser(scanner)

		if node, ok := node.(Event); ok {
			offset := scanner.GetCursor()
			s.buf = s.buf[offset:]
			return node, s.buf, nil
		} else if eof != nil {
			return nil, s.buf, eof
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

	return left, unmarshalEvent(event, v)
}

const defaultReadSize = 64

// NewStreamScanner scans a [io.Reader] for SSEs.
func NewStreamScanner(reader io.Reader, opts ...Opt) *StreamScanner {
	if reader == nil {
		panic("reader cannot be nil")
	}

	s := StreamScanner{
		buf: []byte{},
		r:   reader,
		rs:  defaultReadSize,
	}

	for _, opt := range opts {
		opt(&s)
	}

	if s.rs <= 0 {
		panic("read size must be greater than 0")
	}

	return &s
}

// Opt is an option for configuring a [StreamScanner].
type Opt func(*StreamScanner)

// WithReadSize sets the read size for the [StreamScanner].
func WithReadSize(size int) Opt {
	return func(s *StreamScanner) {
		s.rs = size
	}
}
