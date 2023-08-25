// Package sseparser provides a parser for Server-Sent Events (SSE).
// The SSE specification this package is modeled on can be found here:
// https://html.spec.whatwg.org/multipage/server-sent-events.html
package sseparser

import (
	"fmt"
	"io"

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

// Field is an SSE field: A key and an optional value.
type Field struct {
	Key   string
	Value string
}

// ParseField parses an input into a Field.
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
	key := nodes[0].(string)
	vnodes := nodes[1].([]parsec.ParsecNode)

	if len(vnodes) == 0 {
		return Field{key, ""}
	}

	v := vnodes[0].([]parsec.ParsecNode)[2]

	return Field{key, v.(string)}
}

// Comment is a comment in an SSE event.
type Comment string

// ParseComment parses an input into a Comment.
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

// Event is an SSE event, which is a set of zero or more comments or fields.
type Event []any

// Fields returns the fields in an SSE event.
func (e Event) Fields() []Field {
	fields := make([]Field, 0, len(e))

	for _, item := range e {
		if field, ok := item.(Field); ok {
			fields = append(fields, field)
		}
	}

	return fields
}

// Comments returns the comments in an SSE event.
func (e Event) Comments() []Comment {
	comments := make([]Comment, 0, len(e))

	for _, item := range e {
		if comment, ok := item.(Comment); ok {
			comments = append(comments, comment)
		}
	}

	return comments
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

// Stream is an SSE stream, which is a set of zero or more events.
type Stream []Event

// ParseStream parses an input into a Stream.
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

// StreamScanner scans a reader for SSE events.
type StreamScanner struct {
	buf []byte
	r   io.Reader
	rs  int
}

// Next returns the next event in the stream.
func (s *StreamScanner) Next() (Event, bool, error) {
	for {
		b := make([]byte, s.rs)

		eof := false
		n, err := s.r.Read(b)
		if err != nil {
			if err == io.EOF {
				eof = true
			} else {
				return Event{}, false, err
			}
		}

		s.buf = append(s.buf, b[:n]...)

		scanner := parsec.NewScanner(s.buf)

		node, scanner := eventParser(scanner)

		if node, ok := node.(Event); ok {
			offset := scanner.GetCursor()
			s.buf = s.buf[offset:]
			return node, true, nil
		} else if eof {
			return Event{}, false, nil
		} else {
			continue
		}
	}
}

// NewStreamScanner scans a reader for SSE events.
func NewStreamScanner(reader io.Reader) *StreamScanner {
	return &StreamScanner{
		buf: []byte{},
		r:   reader,
		rs:  64,
	}
}
